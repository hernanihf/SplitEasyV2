package handler

import (
	"net/http"
	"net/url"
	"spliteasy/internal/config"
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
// @Description  Handles the callback from Google OAuth2, exchanges authorization code for a user JWT, then redirects to the frontend with the token as a query param.
// @Tags         auth
// @Param        state  query     string  true  "OAuth state validation"
// @Param        code   query     string  true  "Authorization code"
// @Success      307    "Temporary Redirect to FRONTEND_REDIRECT_URL?token=..."
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

	redirectURL, err := url.Parse(config.FrontendRedirectURL)
	if err != nil {
		http.Error(w, "invalid frontend redirect url: "+err.Error(), http.StatusInternalServerError)
		return
	}
	query := redirectURL.Query()
	query.Set("token", token)
	redirectURL.RawQuery = query.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusTemporaryRedirect)
}
