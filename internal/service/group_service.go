package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

// Sentinel errors that handlers map to HTTP status codes via errors.Is.
var (
	ErrGroupNotFound  = errors.New("group not found")
	ErrNotGroupMember = errors.New("only group members can share an invite")
)

type GroupService interface {
	CreateGroup(ctx context.Context, name, emoji string, creatorID uint) (*domain.Group, error)
	GetGroup(ctx context.Context, id uint) (*domain.Group, error)
	ListGroupsForUser(ctx context.Context, userID uint) ([]domain.Group, error)
	GetInviteToken(ctx context.Context, groupID, userID uint) (string, error)
	JoinGroup(ctx context.Context, token string, userID uint) (*domain.Group, error)
	VerifyMembership(ctx context.Context, groupID, userID uint) error
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

func (s *groupService) CreateGroup(ctx context.Context, name, emoji string, creatorID uint) (*domain.Group, error) {
	if name == "" {
		return nil, errors.New("group name is required")
	}

	creator, err := s.userRepo.GetByID(ctx, creatorID)
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

	err = s.groupRepo.Create(ctx, group)
	if err != nil {
		return nil, err
	}

	return group, nil
}

func (s *groupService) GetGroup(ctx context.Context, id uint) (*domain.Group, error) {
	return s.groupRepo.GetByID(ctx, id)
}

// VerifyMembership returns nil only if userID belongs to the group, so callers
// can authorize access to group-scoped resources. It returns ErrGroupNotFound
// or ErrNotGroupMember otherwise.
func (s *groupService) VerifyMembership(ctx context.Context, groupID, userID uint) error {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return ErrGroupNotFound
	}
	if !isMember(group, userID) {
		return ErrNotGroupMember
	}
	return nil
}

func (s *groupService) ListGroupsForUser(ctx context.Context, userID uint) ([]domain.Group, error) {
	return s.groupRepo.GetByUserID(ctx, userID)
}

// GetInviteToken returns the group's invite token, but only if the requesting
// user is a member. Older groups created before invite tokens existed get one
// generated lazily on first request.
func (s *groupService) GetInviteToken(ctx context.Context, groupID, userID uint) (string, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return "", ErrGroupNotFound
	}
	if !isMember(group, userID) {
		return "", ErrNotGroupMember
	}

	if group.InviteToken == "" {
		token, err := generateInviteToken()
		if err != nil {
			return "", err
		}
		// Conditional write: only the first concurrent caller persists a token.
		// Re-read so every caller returns the token that actually won, instead
		// of its own locally-generated (possibly clobbered) candidate.
		if err := s.groupRepo.SetInviteTokenIfEmpty(ctx, group.ID, token); err != nil {
			return "", err
		}
		updated, err := s.groupRepo.GetByID(ctx, group.ID)
		if err != nil {
			return "", err
		}
		return updated.InviteToken, nil
	}

	return group.InviteToken, nil
}

// JoinGroup adds the user to the group identified by the invite token. It is
// idempotent: joining a group you already belong to is a no-op.
func (s *groupService) JoinGroup(ctx context.Context, token string, userID uint) (*domain.Group, error) {
	if token == "" {
		return nil, errors.New("invite token is required")
	}

	group, err := s.groupRepo.GetByInviteToken(ctx, token)
	if err != nil {
		return nil, errors.New("invalid or expired invite link")
	}

	if err := s.groupRepo.AddMember(ctx, group.ID, userID); err != nil {
		return nil, err
	}

	return group, nil
}
