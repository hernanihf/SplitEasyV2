package service

import (
	"errors"
	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type GroupService interface {
	CreateGroup(name string, creatorID uint) (*domain.Group, error)
	GetGroup(id uint) (*domain.Group, error)
}

type groupService struct {
	groupRepo repository.GroupRepository
	userRepo  repository.UserRepository
}

func NewGroupService(groupRepo repository.GroupRepository, userRepo repository.UserRepository) GroupService {
	return &groupService{groupRepo, userRepo}
}

func (s *groupService) CreateGroup(name string, creatorID uint) (*domain.Group, error) {
	if name == "" {
		return nil, errors.New("group name is required")
	}

	creator, err := s.userRepo.GetByID(creatorID)
	if err != nil {
		return nil, errors.New("creator user not found")
	}

	group := &domain.Group{
		Name:      name,
		CreatedBy: creatorID,
		Members:   []domain.User{*creator},
	}

	err = s.groupRepo.Create(group)
	if err != nil {
		return nil, err
	}

	return group, nil
}

func (s *groupService) GetGroup(id uint) (*domain.Group, error) {
	return s.groupRepo.GetByID(id)
}
