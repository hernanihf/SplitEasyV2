package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

type GroupService interface {
	CreateGroup(name, emoji string, creatorID uint) (*domain.Group, error)
	GetGroup(id uint) (*domain.Group, error)
	ListGroupsForUser(userID uint) ([]domain.Group, error)
	GetInviteToken(groupID, userID uint) (string, error)
	JoinGroup(token string, userID uint) (*domain.Group, error)
}

type groupService struct {
	groupRepo repository.GroupRepository
	userRepo  repository.UserRepository
}

func NewGroupService(groupRepo repository.GroupRepository, userRepo repository.UserRepository) GroupService {
	return &groupService{groupRepo, userRepo}
}

// generateInviteToken returns a random, URL-safe invite token.
func generateInviteToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isMember(group *domain.Group, userID uint) bool {
	for _, member := range group.Members {
		if member.ID == userID {
			return true
		}
	}
	return false
}

func (s *groupService) CreateGroup(name, emoji string, creatorID uint) (*domain.Group, error) {
	if name == "" {
		return nil, errors.New("group name is required")
	}

	creator, err := s.userRepo.GetByID(creatorID)
	if err != nil {
		return nil, errors.New("creator user not found")
	}

	token, err := generateInviteToken()
	if err != nil {
		return nil, err
	}

	if emoji == "" {
		emoji = "💸"
	}

	group := &domain.Group{
		Name:        name,
		Emoji:       emoji,
		CreatedBy:   creatorID,
		InviteToken: token,
		Members:     []domain.User{*creator},
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

func (s *groupService) ListGroupsForUser(userID uint) ([]domain.Group, error) {
	return s.groupRepo.GetByUserID(userID)
}

// GetInviteToken returns the group's invite token, but only if the requesting
// user is a member. Older groups created before invite tokens existed get one
// generated lazily on first request.
func (s *groupService) GetInviteToken(groupID, userID uint) (string, error) {
	group, err := s.groupRepo.GetByID(groupID)
	if err != nil {
		return "", errors.New("group not found")
	}
	if !isMember(group, userID) {
		return "", errors.New("only group members can share an invite")
	}

	if group.InviteToken == "" {
		token, err := generateInviteToken()
		if err != nil {
			return "", err
		}
		if err := s.groupRepo.UpdateInviteToken(group.ID, token); err != nil {
			return "", err
		}
		group.InviteToken = token
	}

	return group.InviteToken, nil
}

// JoinGroup adds the user to the group identified by the invite token. It is
// idempotent: joining a group you already belong to is a no-op.
func (s *groupService) JoinGroup(token string, userID uint) (*domain.Group, error) {
	if token == "" {
		return nil, errors.New("invite token is required")
	}

	group, err := s.groupRepo.GetByInviteToken(token)
	if err != nil {
		return nil, errors.New("invalid or expired invite link")
	}

	if err := s.groupRepo.AddMember(group.ID, userID); err != nil {
		return nil, err
	}

	return group, nil
}
