package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"spliteasy/internal/domain"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
)

type fakeImportService struct {
	previewResult *domain.ImportPreview
	previewErr    error
	gotGroupID    uint

	importResult service.ImportResult
	importErr    error
	gotRows      []domain.ImportRow
	gotMapping   map[string]uint

	exportData     []byte
	exportFilename string
	exportErr      error
}

func (f *fakeImportService) ParsePreview(_ context.Context, groupID uint, _ io.Reader) (*domain.ImportPreview, error) {
	f.gotGroupID = groupID
	return f.previewResult, f.previewErr
}

func (f *fakeImportService) Import(_ context.Context, groupID, _ uint, rows []domain.ImportRow, mapping map[string]uint) (service.ImportResult, error) {
	f.gotGroupID = groupID
	f.gotRows = rows
	f.gotMapping = mapping
	return f.importResult, f.importErr
}

func (f *fakeImportService) ExportGroupCSV(_ context.Context, groupID uint) ([]byte, string, error) {
	f.gotGroupID = groupID
	return f.exportData, f.exportFilename, f.exportErr
}

func newMultipartCSVRequest(t *testing.T, groupID, filename string, content []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID+"/import/preview", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(7)))
	return withURLParam(req, "id", groupID)
}

func TestPreviewImport_Success(t *testing.T) {
	fake := &fakeImportService{previewResult: &domain.ImportPreview{MemberColumns: []string{"A", "B"}, Rows: []domain.ImportRow{{Description: "x"}}}}
	h := NewImportHandler(fake, fakeGroupServiceForBalance{}, nil)

	req := newMultipartCSVRequest(t, "1", "export.csv", []byte("Fecha,Descripción,Categoría,Coste,Moneda,A,B\n"))
	rec := httptest.NewRecorder()
	h.PreviewImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got domain.ImportPreview
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].Description != "x" {
		t.Errorf("unexpected preview body: %+v", got)
	}
}

func TestPreviewImport_RejectsMissingFile(t *testing.T) {
	fake := &fakeImportService{}
	h := NewImportHandler(fake, fakeGroupServiceForBalance{}, nil)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.Close() //nolint:errcheck

	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/1/import/preview", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(7)))
	req = withURLParam(req, "id", "1")

	rec := httptest.NewRecorder()
	h.PreviewImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPreviewImport_MapsInvalidCSVTo400(t *testing.T) {
	fake := &fakeImportService{previewErr: service.ErrInvalidCSV}
	h := NewImportHandler(fake, fakeGroupServiceForBalance{}, nil)

	req := newMultipartCSVRequest(t, "1", "export.csv", []byte("garbage"))
	rec := httptest.NewRecorder()
	h.PreviewImport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func confirmImportRequest(t *testing.T, fake *fakeImportService, audit *fakeAuditService, groupID string, authUserID uint, body ImportRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/"+groupID+"/import", bytes.NewReader(payload))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(authUserID)))
	req = withURLParam(req, "id", groupID)

	rec := httptest.NewRecorder()
	h := NewImportHandler(fake, fakeGroupServiceForBalance{}, audit)
	h.ConfirmImport(rec, req)
	return rec
}

func TestConfirmImport_Success(t *testing.T) {
	fake := &fakeImportService{importResult: service.ImportResult{Imported: 3, Failed: 1}}
	audit := &fakeAuditService{}
	rec := confirmImportRequest(t, fake, audit, "1", 7, ImportRequest{
		Rows:          []domain.ImportRow{{Description: "x", AmountCents: 100}},
		MemberMapping: map[string]uint{"A": 10},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got ImportResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Imported != 3 || got.Failed != 1 {
		t.Errorf("unexpected response: %+v", got)
	}
	if len(fake.gotRows) != 1 || fake.gotMapping["A"] != 10 {
		t.Errorf("expected rows/mapping passed through, got rows=%+v mapping=%+v", fake.gotRows, fake.gotMapping)
	}
	if len(audit.records) != 1 || audit.records[0].action != domain.AuditActionExpensesImported {
		t.Errorf("expected an audit entry for the import, got %+v", audit.records)
	}
}

func TestConfirmImport_RejectsEmptyRows(t *testing.T) {
	fake := &fakeImportService{}
	rec := confirmImportRequest(t, fake, nil, "1", 7, ImportRequest{Rows: nil, MemberMapping: map[string]uint{}})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConfirmImport_MapsGroupNotEmptyTo409(t *testing.T) {
	fake := &fakeImportService{importErr: service.ErrGroupNotEmpty}
	rec := confirmImportRequest(t, fake, nil, "1", 7, ImportRequest{
		Rows: []domain.ImportRow{{Description: "x"}}, MemberMapping: map[string]uint{},
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConfirmImport_MapsNotMemberTo403(t *testing.T) {
	fake := &fakeImportService{importErr: service.ErrNotGroupMember}
	rec := confirmImportRequest(t, fake, nil, "1", 7, ImportRequest{
		Rows: []domain.ImportRow{{Description: "x"}}, MemberMapping: map[string]uint{},
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConfirmImport_RejectsUnauthenticated(t *testing.T) {
	fake := &fakeImportService{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/1/import", bytes.NewReader([]byte(`{"rows":[{}]}`)))
	req = withURLParam(req, "id", "1")
	rec := httptest.NewRecorder()
	h := NewImportHandler(fake, fakeGroupServiceForBalance{}, nil)
	h.ConfirmImport(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestExportGroupCSV_Success(t *testing.T) {
	fake := &fakeImportService{exportData: []byte("Date,Description,Category,Cost,Currency,Ana\n"), exportFilename: "asado-splitwise.csv"}
	h := NewImportHandler(fake, fakeGroupServiceForBalance{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/1/export.csv", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, float64(7)))
	req = withURLParam(req, "id", "1")
	rec := httptest.NewRecorder()
	h.ExportGroupCSV(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.gotGroupID != 1 {
		t.Errorf("expected group id 1, got %d", fake.gotGroupID)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="asado-splitwise.csv"` {
		t.Errorf("unexpected Content-Disposition: %q", got)
	}
	if rec.Body.String() != "Date,Description,Category,Cost,Currency,Ana\n" {
		t.Errorf("unexpected body: %q", rec.Body.String())
	}
}

func TestExportGroupCSV_RejectsUnauthenticated(t *testing.T) {
	fake := &fakeImportService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/1/export.csv", nil)
	req = withURLParam(req, "id", "1")
	rec := httptest.NewRecorder()
	h := NewImportHandler(fake, fakeGroupServiceForBalance{}, nil)
	h.ExportGroupCSV(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
