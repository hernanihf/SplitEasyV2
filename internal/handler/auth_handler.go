package handler

import (
	"encoding/json"
	"net/http"
	"spliteasy/internal/service"
)

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService}
}

// GoogleLogin godoc
// @Summary      Redirect to Google Login URL
// @Description  Redirects client to Google OAuth2 consent page.
// @Tags         auth
// @Success      307  "Temporary Redirect to Google OAuth"
// @Router       /auth/google/login [get]
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	// Simple state parameter (in production, use a secure random string)
	state := "random-state-string"
	url := h.authService.GetGoogleLoginURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// GoogleCallback godoc
// @Summary      Handle Google Login Callback
// @Description  Handles the callback from Google OAuth2, exchanges authorization code for user JWT token.
// @Tags         auth
// @Param        state  query     string  true  "OAuth state validation"
// @Param        code   query     string  true  "Authorization code"
// @Success      200    {object}  map[string]string "JSON token response"
// @Failure      400    {string}  string "Bad Request"
// @Failure      500    {string}  string "Internal Server Error"
// @Router       /auth/google/callback [get]
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state != "random-state-string" {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code not found", http.StatusBadRequest)
		return
	}

	token, err := h.authService.HandleGoogleCallback(code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}
