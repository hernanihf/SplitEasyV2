package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"spliteasy/internal/domain"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"

	"github.com/go-chi/chi/v5"
)

// maxImportCSVBytes caps the uploaded file — generous for even a multi-year,
// many-member export (the reference Splitwise export this was built against
// is ~40KB for 2.5 years of a 2-person group).
const maxImportCSVBytes = 5 << 20 // 5MB

type ImportHandler struct {
	importService service.ImportService
	groupService  service.GroupService
	auditService  service.AuditService
}

func NewImportHandler(importService service.ImportService, groupService service.GroupService, auditService service.AuditService) *ImportHandler {
	return &ImportHandler{importService, groupService, auditService}
}

// PreviewImport godoc
// @Summary      Preview a Splitwise-style expense CSV import
// @Description  Parses an uploaded CSV (e.g. a Splitwise export) into a preview — detected member columns and structured rows — without creating anything. The frontend uses this to let the user map each column to a real group member before confirming.
// @Tags         groups
// @Accept       multipart/form-data
// @Produce      json
// @Param        id    path      int   true  "Group ID"
// @Param        file  formData  file  true  "Expense history CSV"
// @Success      200   {object}  domain.ImportPreview
// @Failure      400   {string}  string  "Bad Request"
// @Failure      401   {string}  string  "Unauthorized"
// @Failure      403   {string}  string  "Forbidden"
// @Failure      404   {string}  string  "Not Found"
// @Security     JWT
// @Router       /groups/{id}/import/preview [post]
func (h *ImportHandler) PreviewImport(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !authorizeGroupAccess(w, r, h.groupService, uint(groupID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportCSVBytes+1<<16)
	if err := r.ParseMultipartForm(maxImportCSVBytes + 1<<16); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	preview, err := h.importService.ParsePreview(r.Context(), uint(groupID), file)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGroupNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, service.ErrInvalidCSV):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			internalError(w, "failed to parse import CSV", err)
		}
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

type ImportRequest struct {
	Rows []domain.ImportRow `json:"rows"`
	// MemberMapping maps a CSV member-column name (from the preview's
	// member_columns) to the group member's user ID it corresponds to.
	MemberMapping map[string]uint `json:"member_mapping"`
}

type ImportResponse struct {
	Imported int `json:"imported"`
	Failed   int `json:"failed"`
}

// ConfirmImport godoc
// @Summary      Import expenses from a previously-parsed CSV
// @Description  Creates one expense per row using memberMapping to resolve each row's payer and splits, preserving the original dates. Only allowed on a group with no expenses or settlements yet — this is a one-time history migration, not an ongoing sync.
// @Tags         groups
// @Accept       json
// @Produce      json
// @Param        id      path      int             true  "Group ID"
// @Param        import  body      ImportRequest   true  "Parsed rows and column-to-member mapping"
// @Success      200     {object}  ImportResponse
// @Failure      400     {string}  string  "Bad Request"
// @Failure      401     {string}  string  "Unauthorized"
// @Failure      403     {string}  string  "Forbidden"
// @Failure      404     {string}  string  "Not Found"
// @Failure      409     {string}  string  "Conflict"
// @Security     JWT
// @Router       /groups/{id}/import [post]
func (h *ImportHandler) ConfirmImport(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Rows) == 0 {
		http.Error(w, "no rows to import", http.StatusBadRequest)
		return
	}

	result, err := h.importService.Import(r.Context(), uint(groupID), userID, req.Rows, req.MemberMapping)
	switch {
	case err == nil:
		recordAudit(
			r.Context(), h.auditService, uint(groupID), userID,
			domain.AuditActionExpensesImported, domain.AuditEntityGroup, uint(groupID),
			strconv.Itoa(result.Imported)+" expenses imported",
		)
		writeJSON(w, http.StatusOK, ImportResponse{Imported: result.Imported, Failed: result.Failed})
	case errors.Is(err, service.ErrGroupNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, service.ErrNotGroupMember):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, service.ErrGroupNotEmpty):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// ExportGroupCSV godoc
// @Summary      Export a group's history as a Splitwise-compatible CSV
// @Description  Downloads every expense and settlement in the group as a CSV in the same column shape Splitwise's own export uses — opens directly in Splitwise, or back into this app via the CSV import above.
// @Tags         groups
// @Produce      text/csv
// @Param        id  path  int  true  "Group ID"
// @Success      200  {file}    file
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Security     JWT
// @Router       /groups/{id}/export.csv [get]
func (h *ImportHandler) ExportGroupCSV(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !authorizeGroupAccess(w, r, h.groupService, uint(groupID)) {
		return
	}

	data, filename, err := h.importService.ExportGroupCSV(r.Context(), uint(groupID))
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		internalError(w, "failed to export group CSV", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ExportSpendingXLSX godoc
// @Summary      Export a group's category spending as an Excel workbook
// @Description  Downloads a workbook with a "Spending" sheet (category totals plus a native pie chart matching the app's own) and a "Details" sheet (every expense, grouped in that same category order) — scoped to an optional date filter. Pass year+month for a calendar month, from/to (YYYY-MM-DD, "to" inclusive) for an arbitrary range, or nothing for the group's full history.
// @Tags         groups
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param        id     path      int     true   "Group ID"
// @Param        year   query     int     false  "Calendar year — pairs with month"
// @Param        month  query     int     false  "Calendar month, 1-12 — requires year"
// @Param        from   query     string  false  "Range start, YYYY-MM-DD"
// @Param        to     query     string  false  "Range end (inclusive), YYYY-MM-DD"
// @Success      200  {file}    file
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Security     JWT
// @Router       /groups/{id}/spending.xlsx [get]
func (h *ImportHandler) ExportSpendingXLSX(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !authorizeGroupAccess(w, r, h.groupService, uint(groupID)) {
		return
	}

	from, to, err := parseSpendingDateFilter(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, filename, err := h.importService.ExportSpendingXLSX(r.Context(), uint(groupID), from, to)
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		internalError(w, "failed to export spending workbook", err)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data) // #nosec G705 -- data is a generated xlsx binary, not reflected query text; gosec's taint tracker just can't see that far through ExportSpendingXLSX
}

// parseSpendingDateFilter reads either "year"+"month" (a calendar month) or
// "from"/"to" (an arbitrary range, "to" inclusive through end of day) into a
// from/to bound pair — every param is optional, and year+month wins if both
// styles are somehow present at once.
func parseSpendingDateFilter(q url.Values) (from, to *time.Time, err error) {
	if yearStr := q.Get("year"); yearStr != "" {
		year, yearErr := strconv.Atoi(yearStr)
		month, monthErr := strconv.Atoi(q.Get("month"))
		if yearErr != nil || monthErr != nil || month < 1 || month > 12 {
			return nil, nil, errors.New("invalid year/month")
		}
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
		return &start, &end, nil
	}

	if fromStr := q.Get("from"); fromStr != "" {
		t, parseErr := time.Parse("2006-01-02", fromStr)
		if parseErr != nil {
			return nil, nil, errors.New("invalid from")
		}
		from = &t
	}
	if toStr := q.Get("to"); toStr != "" {
		t, parseErr := time.Parse("2006-01-02", toStr)
		if parseErr != nil {
			return nil, nil, errors.New("invalid to")
		}
		endOfDay := t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		to = &endOfDay
	}
	return from, to, nil
}
