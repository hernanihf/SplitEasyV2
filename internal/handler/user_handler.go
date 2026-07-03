package handler

import (
	"encoding/json"
	"net/http"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
)

type UserHandler struct {
	userService service.UserService
	pushService service.PushService
}

func NewUserHandler(userService service.UserService, pushService service.PushService) *UserHandler {
	return &UserHandler{userService, pushService}
}

// GetMe godoc
// @Summary      Get the authenticated user
// @Description  Returns the profile of the currently authenticated user.
// @Tags         users
// @Produce      json
// @Success      200  {object}  domain.User
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      404  {string}  string  "Not Found"
// @Security     JWT
// @Router       /users/me [get]
func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	user, err := h.userService.GetUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

type SetPushPreferenceRequest struct {
	PushEnabled bool `json:"push_enabled" example:"true"`
}

// SetPushPreference godoc
// @Summary      Enable or disable push notifications
// @Description  Sets whether the authenticated user receives push notifications for group activity. Doesn't touch existing device subscriptions — disabling just stops sends, so re-enabling later doesn't need the browser permission prompt again.
// @Tags         users
// @Accept       json
// @Param        preference  body  SetPushPreferenceRequest  true  "Push preference"
// @Success      204  "No Content"
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /users/me/push-preference [patch]
func (h *UserHandler) SetPushPreference(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	var req SetPushPreferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.pushService.SetPushEnabled(r.Context(), userID, req.PushEnabled); err != nil {
		internalError(w, "failed to update push preference", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
