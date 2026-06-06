package config

import (
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	GoogleOAuthConfig *oauth2.Config
	JWTSecret         []byte
)

func InitAuth() {
	GoogleOAuthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:8080/api/v1/auth/google/callback",
		ClientID:     getEnv("GOOGLE_CLIENT_ID", "mock-client-id"),
		ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", "mock-client-secret"),
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	JWTSecret = []byte(getEnv("JWT_SECRET", "super-secret-key-change-me"))
}
