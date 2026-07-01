package handler

import (
	"net/http"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
)

type UserHandler struct {
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService}
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
