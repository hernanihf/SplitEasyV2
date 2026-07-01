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

type CommentHandler struct {
	commentService service.CommentService
	expenseService service.ExpenseService
	balanceService service.BalanceService
	groupService   service.GroupService
}

func NewCommentHandler(
	commentService service.CommentService,
	expenseService service.ExpenseService,
	balanceService service.BalanceService,
	groupService service.GroupService,
) *CommentHandler {
	return &CommentHandler{commentService, expenseService, balanceService, groupService}
}

type AddCommentRequest struct {
	Body string `json:"body" example:"Can we split this differently?"`
}

// AddExpenseComment godoc
// @Summary      Comment on an expense
// @Description  Adds a comment to an expense. Any member of the expense's group may comment.
// @Tags         comments
// @Accept       json
// @Produce      json
// @Param        id       path      int                 true  "Expense ID"
// @Param        comment  body      AddCommentRequest   true  "Comment body"
// @Success      201      {object}  domain.Comment
// @Failure      400      {string}  string  "Bad Request"
// @Failure      401      {string}  string  "Unauthorized"
// @Failure      403      {string}  string  "Forbidden"
// @Failure      404      {string}  string  "Not Found"
// @Failure      500      {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /expenses/{id}/comments [post]
func (h *CommentHandler) AddExpenseComment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	expenseID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	expense, err := h.expenseService.GetExpense(r.Context(), uint(expenseID))
	if err != nil {
		if errors.Is(err, service.ErrExpenseNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			internalError(w, "failed to get expense", err)
		}
		return
	}
	if !authorizeGroupAccess(w, r, h.groupService, expense.GroupID) {
		return
	}

	var req AddCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateMaxLen("comment", req.Body, maxCommentLen); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	comment, err := h.commentService.AddExpenseComment(r.Context(), uint(expenseID), userID, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// ListExpenseComments godoc
// @Summary      List an expense's comments
// @Description  Returns every comment on an expense, oldest first. Any member of the expense's group may view them.
// @Tags         comments
// @Produce      json
// @Param        id   path      int  true  "Expense ID"
// @Success      200  {array}   domain.Comment
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /expenses/{id}/comments [get]
func (h *CommentHandler) ListExpenseComments(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	expenseID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	expense, err := h.expenseService.GetExpense(r.Context(), uint(expenseID))
	if err != nil {
		if errors.Is(err, service.ErrExpenseNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			internalError(w, "failed to get expense", err)
		}
		return
	}
	if !authorizeGroupAccess(w, r, h.groupService, expense.GroupID) {
		return
	}

	comments, err := h.commentService.ListExpenseComments(r.Context(), uint(expenseID))
	if err != nil {
		internalError(w, "failed to list expense comments", err)
		return
	}

	writeJSON(w, http.StatusOK, comments)
}

// AddSettlementComment godoc
// @Summary      Comment on a settlement
// @Description  Adds a comment to a settlement. Any member of the settlement's group may comment.
// @Tags         comments
// @Accept       json
// @Produce      json
// @Param        id       path      int                 true  "Settlement ID"
// @Param        comment  body      AddCommentRequest   true  "Comment body"
// @Success      201      {object}  domain.Comment
// @Failure      400      {string}  string  "Bad Request"
// @Failure      401      {string}  string  "Unauthorized"
// @Failure      403      {string}  string  "Forbidden"
// @Failure      404      {string}  string  "Not Found"
// @Failure      500      {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /settlements/{id}/comments [post]
func (h *CommentHandler) AddSettlementComment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	settlementID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	settlement, err := h.balanceService.GetSettlement(r.Context(), uint(settlementID))
	if err != nil {
		if errors.Is(err, service.ErrSettlementNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			internalError(w, "failed to get settlement", err)
		}
		return
	}
	if !authorizeGroupAccess(w, r, h.groupService, settlement.GroupID) {
		return
	}

	var req AddCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateMaxLen("comment", req.Body, maxCommentLen); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	comment, err := h.commentService.AddSettlementComment(r.Context(), uint(settlementID), userID, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// ListSettlementComments godoc
// @Summary      List a settlement's comments
// @Description  Returns every comment on a settlement, oldest first. Any member of the settlement's group may view them.
// @Tags         comments
// @Produce      json
// @Param        id   path      int  true  "Settlement ID"
// @Success      200  {array}   domain.Comment
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /settlements/{id}/comments [get]
func (h *CommentHandler) ListSettlementComments(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	settlementID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	settlement, err := h.balanceService.GetSettlement(r.Context(), uint(settlementID))
	if err != nil {
		if errors.Is(err, service.ErrSettlementNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			internalError(w, "failed to get settlement", err)
		}
		return
	}
	if !authorizeGroupAccess(w, r, h.groupService, settlement.GroupID) {
		return
	}

	comments, err := h.commentService.ListSettlementComments(r.Context(), uint(settlementID))
	if err != nil {
		internalError(w, "failed to list settlement comments", err)
		return
	}

	writeJSON(w, http.StatusOK, comments)
}

// DeleteComment godoc
// @Summary      Delete a comment
// @Description  Soft-deletes a comment. Only its author may delete it.
// @Tags         comments
// @Param        id   path  int  true  "Comment ID"
// @Success      204  "No Content"
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Failure      404  {string}  string  "Not Found"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /comments/{id} [delete]
func (h *CommentHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	commentID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	switch err := h.commentService.DeleteComment(r.Context(), uint(commentID), userID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, service.ErrCommentNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, service.ErrNotCommentAuthor):
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		internalError(w, "failed to delete comment", err)
	}
}
