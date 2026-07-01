package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type ExpenseHandler struct {
	expenseService service.ExpenseService
	groupService   service.GroupService
}

func NewExpenseHandler(expenseService service.ExpenseService, groupService service.GroupService) *ExpenseHandler {
	return &ExpenseHandler{expenseService, groupService}
}

// isCallerInSplits reports whether userID is one of the split participants.
// An empty split list only ever means "equal split among the whole group"
// (every other method requires an explicit, non-empty list) — since
// authorizeGroupAccess already confirmed the caller is a group member,
// that implicitly includes them too.
func isCallerInSplits(userID uint, splits []SplitInputRequest) bool {
	if len(splits) == 0 {
		return true
	}
	for _, s := range splits {
		if s.UserID == userID {
			return true
		}
	}
	return false
}

type SplitInputRequest struct {
	UserID uint    `json:"user_id" example:"2"`
	Value  float64 `json:"value" example:"50"`
}

type ItemInputRequest struct {
	Description string `json:"description" example:"Burger"`
	Amount      int64  `json:"amount" example:"1500"`
	UserIDs     []uint `json:"user_ids"`
}

type AddExpenseRequest struct {
	GroupID     uint                `json:"group_id" example:"1"`
	PaidByID    uint                `json:"paid_by_id" example:"1"`
	Description string              `json:"description" example:"Dinner"`
	Amount      int64               `json:"amount" example:"12050"`
	SplitMethod string              `json:"split_method" example:"equal" enums:"equal,percentage,fixed,shares"`
	Splits      []SplitInputRequest `json:"splits"`
	Items       []ItemInputRequest  `json:"items"`
}

// AddExpense godoc
// @Summary      Add an expense
// @Description  Adds a new expense to a group and splits it. split_method can be "equal" (default; splits among all members, or the given subset), "percentage" (splits values must add up to 100), "fixed" (splits values must add up to amount), or "shares" (splits proportionally to relative weights).
// @Tags         expenses
// @Accept       json
// @Produce      json
// @Param        expense  body      AddExpenseRequest  true  "Expense Info"
// @Success      201      {object}  domain.Expense
// @Failure      400      {string}  string  "Bad Request"
// @Failure      401      {string}  string  "Unauthorized"
// @Failure      403      {string}  string  "Forbidden"
// @Failure      404      {string}  string  "Not Found"
// @Failure      500      {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /expenses [post]
func (h *ExpenseHandler) AddExpense(w http.ResponseWriter, r *http.Request) {
	var req AddExpenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateMaxLen("description", req.Description, maxDescriptionLen); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, it := range req.Items {
		if err := validateMaxLen("item description", it.Description, maxDescriptionLen); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if !authorizeGroupAccess(w, r, h.groupService, req.GroupID) {
		return
	}

	// You can log that someone else paid (a real flow: "my roommate covered
	// dinner, let me split it"), but only if you're also part of the split —
	// otherwise any group member could fabricate an expense entirely between
	// two other people, with no way to edit or delete it afterward.
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}
	if userID != req.PaidByID && !isCallerInSplits(userID, req.Splits) {
		http.Error(w, "you must be the payer or one of the split participants", http.StatusForbidden)
		return
	}

	splitInputs := make([]service.SplitInput, len(req.Splits))
	for i, s := range req.Splits {
		splitInputs[i] = service.SplitInput{UserID: s.UserID, Value: s.Value}
	}

	items := make([]service.ItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.ItemInput{Description: it.Description, Amount: it.Amount, UserIDs: it.UserIDs}
	}

	expense, err := h.expenseService.AddExpense(
		r.Context(),
		req.GroupID,
		req.PaidByID,
		req.Description,
		req.Amount,
		service.SplitMethod(req.SplitMethod),
		splitInputs,
		items,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, expense)
}

type UpdateExpenseRequest struct {
	PaidByID    uint                `json:"paid_by_id" example:"1"`
	Description string              `json:"description" example:"Dinner"`
	Amount      int64               `json:"amount" example:"12050"`
	SplitMethod string              `json:"split_method" example:"equal" enums:"equal,percentage,fixed,shares"`
	Splits      []SplitInputRequest `json:"splits"`
	Items       []ItemInputRequest  `json:"items"`
}

// UpdateExpense godoc
// @Summary      Edit an expense
// @Description  Replaces an expense's payer, description, amount, split, and items. Only the current payer or a current split participant may edit it.
// @Tags         expenses
// @Accept       json
// @Produce      json
// @Param        id       path      int                   true  "Expense ID"
// @Param        expense  body      UpdateExpenseRequest  true  "Updated expense info"
// @Success      200      {object}  domain.Expense
// @Failure      400      {string}  string  "Bad Request"
// @Failure      401      {string}  string  "Unauthorized"
// @Failure      403      {string}  string  "Forbidden"
// @Failure      404      {string}  string  "Not Found"
// @Security     JWT
// @Router       /expenses/{id} [put]
func (h *ExpenseHandler) UpdateExpense(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	expenseID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req UpdateExpenseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateMaxLen("description", req.Description, maxDescriptionLen); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, it := range req.Items {
		if err := validateMaxLen("item description", it.Description, maxDescriptionLen); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	splitInputs := make([]service.SplitInput, len(req.Splits))
	for i, s := range req.Splits {
		splitInputs[i] = service.SplitInput{UserID: s.UserID, Value: s.Value}
	}
	items := make([]service.ItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = service.ItemInput{Description: it.Description, Amount: it.Amount, UserIDs: it.UserIDs}
	}

	expense, err := h.expenseService.UpdateExpense(
		r.Context(),
		uint(expenseID),
		userID,
		req.PaidByID,
		req.Description,
		req.Amount,
		service.SplitMethod(req.SplitMethod),
		splitInputs,
		items,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrExpenseNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, service.ErrNotExpenseParty):
			http.Error(w, err.Error(), http.StatusForbidden)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	writeJSON(w, http.StatusOK, expense)
}

// DeleteExpense godoc
// @Summary      Delete an expense
// @Description  Soft-deletes an expense. Only the payer or a split participant may delete it.
// @Tags         expenses
// @Param        id   path  int  true  "Expense ID"
// @Success      204  "No Content"
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /expenses/{id} [delete]
func (h *ExpenseHandler) DeleteExpense(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	expenseID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	switch err := h.expenseService.DeleteExpense(r.Context(), uint(expenseID), userID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, service.ErrExpenseNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, service.ErrNotExpenseParty):
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		internalError(w, "failed to delete expense", err)
	}
}

// GetGroupExpenses godoc
// @Summary      Get expenses of a group
// @Description  Retrieves list of expenses for a specific group.
// @Tags         expenses
// @Produce      json
// @Param        groupId  path      int  true  "Group ID"
// @Success      200      {array}   domain.Expense
// @Failure      400      {string}  string  "Bad Request"
// @Failure      401      {string}  string  "Unauthorized"
// @Failure      403      {string}  string  "Forbidden"
// @Failure      404      {string}  string  "Not Found"
// @Failure      500      {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /groups/{groupId}/expenses [get]
func (h *ExpenseHandler) GetGroupExpenses(w http.ResponseWriter, r *http.Request) {
	groupIDStr := chi.URLParam(r, "groupId")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	if !authorizeGroupAccess(w, r, h.groupService, uint(groupID)) {
		return
	}

	expenses, err := h.expenseService.GetGroupExpenses(r.Context(), uint(groupID))
	if err != nil {
		internalError(w, "failed to get group expenses", err)
		return
	}

	writeJSON(w, http.StatusOK, expenses)
}
