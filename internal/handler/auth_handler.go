package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"

	"spliteasy/internal/config"
	"spliteasy/internal/service"
)

const oauthStateCookie = "oauth_state"

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService}
}

// generateState returns a cryptographically random, URL-safe state string for
// CSRF protection of the OAuth flow.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// isHTTPS reports whether the request reached us over HTTPS, accounting for
// TLS-terminating proxies (e.g. Render) that forward plain HTTP with the
// X-Forwarded-Proto header set.
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// GoogleLogin godoc
// @Summary      Redirect to Google Login URL
// @Description  Redirects client to Google OAuth2 consent page.
// @Tags         auth
// @Success      307  "Temporary Redirect to Google OAuth"
// @Router       /auth/google/login [get]
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	// Generate a random state and stash it in a short-lived cookie so the
	// callback can verify the request originated from this login flow (CSRF).
	state, err := generateState()
	if err != nil {
		http.Error(w, "failed to generate oauth state", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes — enough to complete the consent screen
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode, // sent on the top-level redirect back from Google
	})

	url := h.authService.GetGoogleLoginURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// GoogleCallback godoc
// @Summary      Handle Google Login Callback
// @Description  Handles the callback from Google OAuth2, exchanges authorization code for a user JWT, then redirects to the frontend with the token in the URL fragment (#token=...). The fragment is never sent to the frontend server, so the JWT does not appear in access logs, CDN logs, or Referer headers.
// @Tags         auth
// @Param        state  query     string  true  "OAuth state validation"
// @Param        code   query     string  true  "Authorization code"
// @Success      307    "Temporary Redirect to FRONTEND_REDIRECT_URL#token=..."
// @Failure      400    {string}  string "Bad Request"
// @Failure      500    {string}  string "Internal Server Error"
// @Router       /auth/google/callback [get]
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	// Validate the state param against the cookie set at login time (CSRF).
	stateParam := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value == "" ||
		subtle.ConstantTimeCompare([]byte(stateParam), []byte(stateCookie.Value)) != 1 {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	// State is single-use: clear the cookie now that it has been consumed.
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code not found", http.StatusBadRequest)
		return
	}

	token, err := h.authService.HandleGoogleCallback(r.Context(), code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	redirectURL, err := url.Parse(config.FrontendRedirectURL)
	if err != nil {
		http.Error(w, "invalid frontend redirect url: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Embed the JWT in the URL fragment instead of a query parameter.
	// The fragment (#) is processed entirely by the browser — it is never
	// transmitted to the frontend server, so the token will not appear in
	// access logs, CDN/proxy logs, or downstream Referer headers.
	// The frontend reads it via: new URLSearchParams(window.location.hash.slice(1)).get("token")
	redirectURL.Fragment = "token=" + token

	http.Redirect(w, r, redirectURL.String(), http.StatusTemporaryRedirect)
}
