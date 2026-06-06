package handler

import (
	"encoding/json"
	"net/http"
	"spliteasy/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type BalanceHandler struct {
	balanceService service.BalanceService
}

func NewBalanceHandler(balanceService service.BalanceService) *BalanceHandler {
	return &BalanceHandler{balanceService}
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

	debts, err := h.balanceService.CalculateGroupDebts(uint(groupID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(debts)
}
