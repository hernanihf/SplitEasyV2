package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
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
// @Description  Handles the callback from Google OAuth2, exchanges authorization code for a session, then redirects to the frontend with the access and refresh tokens in the URL fragment (#access_token=...&refresh_token=...). The fragment is never sent to the frontend server, so neither token appears in access logs, CDN logs, or Referer headers.
// @Tags         auth
// @Param        state  query     string  true  "OAuth state validation"
// @Param        code   query     string  true  "Authorization code"
// @Success      307    "Temporary Redirect to FRONTEND_REDIRECT_URL#access_token=...&refresh_token=..."
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

	accessToken, refreshToken, err := h.authService.HandleGoogleCallback(r.Context(), code)
	if err != nil {
		internalError(w, "google oauth callback failed", err)
		return
	}

	redirectURL, err := url.Parse(config.FrontendRedirectURL)
	if err != nil {
		internalError(w, "invalid FRONTEND_REDIRECT_URL", err)
		return
	}

	// Embed both tokens in the URL fragment instead of a query parameter.
	// The fragment (#) is processed entirely by the browser — it is never
	// transmitted to the frontend server, so neither token appears in access
	// logs, CDN/proxy logs, or downstream Referer headers.
	// The frontend reads it via: new URLSearchParams(window.location.hash.slice(1))
	fragment := url.Values{}
	fragment.Set("access_token", accessToken)
	fragment.Set("refresh_token", refreshToken)
	redirectURL.Fragment = fragment.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusTemporaryRedirect)
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Refresh godoc
// @Summary      Refresh a session
// @Description  Exchanges a valid, unused refresh token for a new access token and a new refresh token (the old refresh token stops working, whether or not this call succeeds).
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RefreshRequest  true  "Refresh token"
// @Success      200   {object}  TokenResponse
// @Failure      400   {string}  string "Bad Request"
// @Failure      401   {string}  string "Unauthorized"
// @Router       /auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	accessToken, refreshToken, err := h.authService.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		http.Error(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, TokenResponse{AccessToken: accessToken, RefreshToken: refreshToken})
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout godoc
// @Summary      Log out
// @Description  Revokes a refresh token so it can no longer be exchanged for a new session. Doesn't require a valid access token — a client whose access token has already expired still needs to be able to revoke its refresh token.
// @Tags         auth
// @Accept       json
// @Param        body  body  LogoutRequest  true  "Refresh token to revoke"
// @Success      204   "No Content"
// @Failure      400   {string}  string "Bad Request"
// @Router       /auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	if err := h.authService.Logout(r.Context(), req.RefreshToken); err != nil {
		internalError(w, "failed to log out", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
