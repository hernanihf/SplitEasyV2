package service

import (
	"context"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type UserService interface {
	GetUser(ctx context.Context, id uint) (*domain.User, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo}
}

func (s *userService) GetUser(ctx context.Context, id uint) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}
