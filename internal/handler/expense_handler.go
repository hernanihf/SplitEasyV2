package handler

import (
	"encoding/json"
	"net/http"
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

type SplitInputRequest struct {
	UserID uint    `json:"user_id" example:"2"`
	Value  float64 `json:"value" example:"50"`
}

type AddExpenseRequest struct {
	GroupID     uint                `json:"group_id" example:"1"`
	PaidByID    uint                `json:"paid_by_id" example:"1"`
	Description string              `json:"description" example:"Dinner"`
	Amount      float64             `json:"amount" example:"120.50"`
	SplitMethod string              `json:"split_method" example:"equal" enums:"equal,percentage,fixed,shares"`
	Splits      []SplitInputRequest `json:"splits"`
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

	if !authorizeGroupAccess(w, r, h.groupService, req.GroupID) {
		return
	}

	splitInputs := make([]service.SplitInput, len(req.Splits))
	for i, s := range req.Splits {
		splitInputs[i] = service.SplitInput{UserID: s.UserID, Value: s.Value}
	}

	expense, err := h.expenseService.AddExpense(
		req.GroupID,
		req.PaidByID,
		req.Description,
		req.Amount,
		service.SplitMethod(req.SplitMethod),
		splitInputs,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(expense)
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

	expenses, err := h.expenseService.GetGroupExpenses(uint(groupID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(expenses)
}
