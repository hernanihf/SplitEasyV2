package handler

import (
	"encoding/json"
	"net/http"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type BalanceHandler struct {
	balanceService service.BalanceService
	groupService   service.GroupService
}

func NewBalanceHandler(balanceService service.BalanceService, groupService service.GroupService) *BalanceHandler {
	return &BalanceHandler{balanceService, groupService}
}

// GetGroupBalances godoc
// @Summary      Get balances/debts of a group
// @Description  Calculates the debts within a group to settle accounts.
// @Tags         groups
// @Produce      json
// @Param        id   path      int  true  "Group ID"
// @Success      200  {array}   domain.Debt
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /groups/{id}/balances [get]
func (h *BalanceHandler) GetGroupBalances(w http.ResponseWriter, r *http.Request) {
	groupIDStr := chi.URLParam(r, "id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	if !authorizeGroupAccess(w, r, h.groupService, uint(groupID)) {
		return
	}

	debts, err := h.balanceService.CalculateGroupDebts(r.Context(), uint(groupID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, debts)
}

// ListSettlements godoc
// @Summary      List a group's settlements
// @Description  Returns every recorded payment in the group for the unified history view.
// @Tags         groups
// @Produce      json
// @Param        id   path      int  true  "Group ID"
// @Success      200  {array}   domain.Settlement
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /groups/{id}/settlements [get]
func (h *BalanceHandler) ListSettlements(w http.ResponseWriter, r *http.Request) {
	groupIDStr := chi.URLParam(r, "id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	if !authorizeGroupAccess(w, r, h.groupService, uint(groupID)) {
		return
	}

	settlements, err := h.balanceService.ListSettlements(r.Context(), uint(groupID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, settlements)
}

type SettleDebtRequest struct {
	FromUserID uint  `json:"from_user_id" example:"2"`
	ToUserID   uint  `json:"to_user_id" example:"1"`
	Amount     int64 `json:"amount" example:"5000"`
}

// SettleDebt godoc
// @Summary      Settle a debt
// @Description  Records a payment between two group members, reducing their outstanding balance.
// @Tags         groups
// @Accept       json
// @Produce      json
// @Param        id        path      int                true  "Group ID"
// @Param        settlement body     SettleDebtRequest  true  "Settlement Info"
// @Success      201       {object}  domain.Settlement
// @Failure      400       {string}  string  "Bad Request"
// @Failure      401       {string}  string  "Unauthorized"
// @Failure      403       {string}  string  "Forbidden"
// @Failure      500       {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /groups/{id}/settlements [post]
func (h *BalanceHandler) SettleDebt(w http.ResponseWriter, r *http.Request) {
	groupIDStr := chi.URLParam(r, "id")
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	if !authorizeGroupAccess(w, r, h.groupService, uint(groupID)) {
		return
	}

	var req SettleDebtRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// A settlement can only be recorded by one of its two parties (the payer
	// confirming they paid, or the payee confirming they got paid) — otherwise
	// any group member could mark an arbitrary pair's debt as settled without
	// either of them being involved.
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}
	if req.FromUserID != userID && req.ToUserID != userID {
		http.Error(w, "you must be a party to the settlement", http.StatusForbidden)
		return
	}

	settlement, err := h.balanceService.SettleDebt(r.Context(), uint(groupID), req.FromUserID, req.ToUserID, req.Amount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, settlement)
}
