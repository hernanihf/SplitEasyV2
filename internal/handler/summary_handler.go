package handler

import (
	"net/http"

	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
)

type SummaryHandler struct {
	summaryService service.SummaryService
}

func NewSummaryHandler(summaryService service.SummaryService) *SummaryHandler {
	return &SummaryHandler{summaryService}
}

// GetHome godoc
// @Summary      Home summary
// @Description  Returns the authenticated user's overall balance and per-group net balances.
// @Tags         summary
// @Produce      json
// @Success      200  {object}  domain.HomeSummary
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /home [get]
func (h *SummaryHandler) GetHome(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	summary, err := h.summaryService.GetHomeSummary(r.Context(), userID)
	if err != nil {
		internalError(w, "failed to get home summary", err)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// GetActivity godoc
// @Summary      Activity feed
// @Description  Returns recent expenses, settlements and comments across the user's groups, newest first.
// @Tags         summary
// @Produce      json
// @Success      200  {array}   domain.ActivityEvent
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /activity [get]
func (h *SummaryHandler) GetActivity(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	events, err := h.summaryService.GetActivity(r.Context(), userID)
	if err != nil {
		internalError(w, "failed to get activity feed", err)
		return
	}

	writeJSON(w, http.StatusOK, events)
}

type UnreadActivityResponse struct {
	Count int `json:"count"`
}

// GetUnreadActivityCount godoc
// @Summary      Unread activity count
// @Description  Counts activity events (expenses, settlements, comments) since the user last viewed the activity feed, excluding events the user caused themselves — for the tab bar badge.
// @Tags         summary
// @Produce      json
// @Success      200  {object}  UnreadActivityResponse
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /activity/unread-count [get]
func (h *SummaryHandler) GetUnreadActivityCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	count, err := h.summaryService.GetUnreadActivityCount(r.Context(), userID)
	if err != nil {
		internalError(w, "failed to get unread activity count", err)
		return
	}

	writeJSON(w, http.StatusOK, UnreadActivityResponse{Count: count})
}

// MarkActivitySeen godoc
// @Summary      Mark activity as seen
// @Description  Records that the user has viewed the activity feed as of now, clearing the unread badge.
// @Tags         summary
// @Success      204  "No Content"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /activity/seen [post]
func (h *SummaryHandler) MarkActivitySeen(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	if err := h.summaryService.MarkActivitySeen(r.Context(), userID); err != nil {
		internalError(w, "failed to mark activity seen", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
