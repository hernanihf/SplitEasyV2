package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"spliteasy/internal/config"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"

	"github.com/golang-jwt/jwt/v5"
)

// accessTokenTTL is short deliberately: a compromised access token is only
// useful until it naturally expires, and there is no per-request revocation
// check (see RefreshTokenStore doc for why). refreshTokenTTL is long so an
// active user effectively stays signed in — the frontend refreshes silently
// in the background — while a stolen refresh token can be revoked instantly
// via Logout, and even without an explicit logout, the exposure is bounded
// to a single use (Consume deletes it) rather than the token's full lifetime.
const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour
)

type AuthService interface {
	GetGoogleLoginURL(state string) string
	// HandleGoogleCallback exchanges an OAuth code for a signed-in session,
	// returning a short-lived access token and a long-lived refresh token.
	HandleGoogleCallback(ctx context.Context, code string) (accessToken, refreshToken string, err error)
	// RefreshToken exchanges a valid, unused refresh token for a new access
	// token and a new refresh token (rotation: the old one stops working).
	RefreshToken(ctx context.Context, refreshToken string) (newAccessToken, newRefreshToken string, err error)
	// Logout revokes a refresh token so it can no longer be exchanged.
	Logout(ctx context.Context, refreshToken string) error
}

type authService struct {
	userRepo     repository.UserRepository
	refreshStore RefreshTokenStore
}

func NewAuthService(userRepo repository.UserRepository, refreshStore RefreshTokenStore) AuthService {
	return &authService{userRepo, refreshStore}
}

// generateAccessToken signs a short-lived JWT identifying userID.
func generateAccessToken(userID uint, email string) (string, error) {
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"exp":     time.Now().Add(accessTokenTTL).Unix(),
	})
	return jwtToken.SignedString(config.JWTSecret)
}

// googleUserInfoClient has an explicit timeout so a slow/hung response from
// Google can't block the calling goroutine indefinitely (http.DefaultClient,
// which the previous code used implicitly via http.Get, has none).
var googleUserInfoClient = &http.Client{Timeout: 10 * time.Second}

func (s *authService) GetGoogleLoginURL(state string) string {
	return config.GoogleOAuthConfig.AuthCodeURL(state)
}

type googleUser struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (s *authService) HandleGoogleCallback(ctx context.Context, code string) (string, string, error) {
	// 1. Exchange code for token
	token, err := config.GoogleOAuthConfig.Exchange(ctx, code)
	if err != nil {
		return "", "", errors.New("code exchange failed: " + err.Error())
	}

	// 2. Get user info. The access token goes in the Authorization header, not
	// a query param, so it never lands in server/proxy access logs.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", "", errors.New("failed building userinfo request: " + err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	response, err := googleUserInfoClient.Do(req)
	if err != nil {
		return "", "", errors.New("failed getting user info: " + err.Error())
	}
	defer response.Body.Close()

	var gUser googleUser
	err = json.NewDecoder(response.Body).Decode(&gUser)
	if err != nil {
		return "", "", errors.New("failed decoding user info: " + err.Error())
	}

	if gUser.Email == "" {
		return "", "", errors.New("no email provided by google")
	}

	// 3. Find or Create User
	user, err := s.userRepo.GetByEmail(ctx, gUser.Email)
	if err != nil {
		// Assume user doesn't exist, create it
		user = &domain.User{
			Name:      gUser.Name,
			Email:     gUser.Email,
			AvatarURL: gUser.Picture,
		}
		err = s.userRepo.Create(ctx, user)
		if err != nil {
			return "", "", errors.New("failed to create user: " + err.Error())
		}
	} else if user.Name != gUser.Name || user.AvatarURL != gUser.Picture {
		// Keep the profile fresh with what Google reports on each login.
		user.Name = gUser.Name
		user.AvatarURL = gUser.Picture
		if err := s.userRepo.Update(ctx, user); err != nil {
			return "", "", errors.New("failed to update user: " + err.Error())
		}
	}

	// 4. Issue a session: short-lived access token + long-lived refresh token.
	accessToken, err := generateAccessToken(user.ID, user.Email)
	if err != nil {
		return "", "", errors.New("failed to generate token: " + err.Error())
	}
	refreshToken, err := s.refreshStore.Create(ctx, user.ID, refreshTokenTTL)
	if err != nil {
		return "", "", errors.New("failed to create refresh token: " + err.Error())
	}

	return accessToken, refreshToken, nil
}

// RefreshToken exchanges a valid refresh token for a new access token and
// rotates the refresh token itself, so a stolen-and-reused refresh token
// stops working the moment the legitimate client refreshes again.
func (s *authService) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	userID, ok, err := s.refreshStore.Consume(ctx, refreshToken)
	if err != nil {
		return "", "", errors.New("failed to validate refresh token: " + err.Error())
	}
	if !ok {
		return "", "", errors.New("invalid or expired refresh token")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", "", errors.New("user not found")
	}

	accessToken, err := generateAccessToken(user.ID, user.Email)
	if err != nil {
		return "", "", errors.New("failed to generate token: " + err.Error())
	}
	newRefreshToken, err := s.refreshStore.Create(ctx, user.ID, refreshTokenTTL)
	if err != nil {
		return "", "", errors.New("failed to create refresh token: " + err.Error())
	}

	return accessToken, newRefreshToken, nil
}

func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	return s.refreshStore.Revoke(ctx, refreshToken)
}
