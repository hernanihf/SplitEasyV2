package service

import (
	"errors"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type UserService interface {
	CreateUser(name, email string) (*domain.User, error)
	GetUser(id uint) (*domain.User, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo}
}

func (s *userService) CreateUser(name, email string) (*domain.User, error) {
	if name == "" || email == "" {
		return nil, errors.New("name and email are required")
	}

	// Check if user already exists
	existingUser, _ := s.repo.GetByEmail(email)
	if existingUser != nil {
		return nil, errors.New("user with this email already exists")
	}

	user := &domain.User{
		Name:  name,
		Email: email,
	}

	err := s.repo.Create(user)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *userService) GetUser(id uint) (*domain.User, error) {
	return s.repo.GetByID(id)
}
