package handler

import (
	"encoding/json"
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

	summary, err := h.summaryService.GetHomeSummary(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// GetActivity godoc
// @Summary      Activity feed
// @Description  Returns recent expenses and settlements across the user's groups, newest first.
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

	events, err := h.summaryService.GetActivity(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
