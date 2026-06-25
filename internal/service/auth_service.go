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

type AuthService interface {
	GetGoogleLoginURL(state string) string
	HandleGoogleCallback(ctx context.Context, code string) (string, error)
}

type authService struct {
	userRepo repository.UserRepository
}

func NewAuthService(userRepo repository.UserRepository) AuthService {
	return &authService{userRepo}
}

func (s *authService) GetGoogleLoginURL(state string) string {
	return config.GoogleOAuthConfig.AuthCodeURL(state)
}

type googleUser struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func (s *authService) HandleGoogleCallback(ctx context.Context, code string) (string, error) {
	// 1. Exchange code for token
	token, err := config.GoogleOAuthConfig.Exchange(ctx, code)
	if err != nil {
		return "", errors.New("code exchange failed: " + err.Error())
	}

	// 2. Get user info
	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		return "", errors.New("failed getting user info: " + err.Error())
	}
	defer response.Body.Close()

	var gUser googleUser
	err = json.NewDecoder(response.Body).Decode(&gUser)
	if err != nil {
		return "", errors.New("failed decoding user info: " + err.Error())
	}

	if gUser.Email == "" {
		return "", errors.New("no email provided by google")
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
			return "", errors.New("failed to create user: " + err.Error())
		}
	} else if user.Name != gUser.Name || user.AvatarURL != gUser.Picture {
		// Keep the profile fresh with what Google reports on each login.
		user.Name = gUser.Name
		user.AvatarURL = gUser.Picture
		if err := s.userRepo.Update(ctx, user); err != nil {
			return "", errors.New("failed to update user: " + err.Error())
		}
	}

	// 4. Generate JWT
	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"exp":     time.Now().Add(time.Hour * 72).Unix(),
	})

	tokenString, err := jwtToken.SignedString(config.JWTSecret)
	if err != nil {
		return "", errors.New("failed to generate token: " + err.Error())
	}

	return tokenString, nil
}
