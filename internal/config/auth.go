package config

import (
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	GoogleOAuthConfig   *oauth2.Config
	JWTSecret           []byte
	FrontendRedirectURL string
	AllowedOrigins      []string
)

func InitAuth() {
	GoogleOAuthConfig = &oauth2.Config{
		RedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
		ClientID:     getEnv("GOOGLE_CLIENT_ID", "mock-client-id"),
		ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", "mock-client-secret"),
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	JWTSecret = []byte(mustGetEnv("JWT_SECRET"))

	FrontendRedirectURL = getEnv("FRONTEND_REDIRECT_URL", "http://localhost:8081/auth/callback")

	AllowedOrigins = parseOrigins(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:8081"))
}

// parseOrigins splits a comma-separated list of origins (as set via the
// CORS_ALLOWED_ORIGINS env var), trimming whitespace and dropping empties.
func parseOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}
