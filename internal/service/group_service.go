package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"

	"spliteasy/internal/domain"
	"spliteasy/internal/repository"
)

// Sentinel errors that handlers map to HTTP status codes via errors.Is.
var (
	ErrGroupNotFound   = errors.New("group not found")
	ErrNotGroupMember  = errors.New("only group members can share an invite")
	ErrNotGroupCreator = errors.New("only the group's creator can delete it")
)

type GroupService interface {
	CreateGroup(ctx context.Context, name, emoji, currency string, creatorID uint) (*domain.Group, error)
	GetGroup(ctx context.Context, id uint) (*domain.Group, error)
	ListGroupsForUser(ctx context.Context, userID uint) ([]domain.Group, error)
	GetInviteToken(ctx context.Context, groupID, userID uint) (string, error)
	JoinGroup(ctx context.Context, token string, userID uint) (*domain.Group, error)
	VerifyMembership(ctx context.Context, groupID, userID uint) error
	// UpdateGroup changes the group's name and/or emoji — any member may
	// call it, unlike DeleteGroup. Fields left nil are unchanged.
	UpdateGroup(ctx context.Context, groupID, callerID uint, name, emoji *string) (*domain.Group, error)
	// DeleteGroup permanently deletes the group and everything under it
	// (expenses, settlements, comments, and their receipt images). Only the
	// group's creator may do this. Irreversible.
	DeleteGroup(ctx context.Context, groupID, callerID uint) error
}

type groupService struct {
	groupRepo      repository.GroupRepository
	userRepo       repository.UserRepository
	storageService StorageService // nil when Supabase Storage isn't configured
}

func NewGroupService(groupRepo repository.GroupRepository, userRepo repository.UserRepository, storageService StorageService) GroupService {
	return &groupService{groupRepo, userRepo, storageService}
}

// generateInviteToken returns a random, URL-safe invite token.
func generateInviteToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *groupService) CreateGroup(ctx context.Context, name, emoji, currency string, creatorID uint) (*domain.Group, error) {
	if name == "" {
		return nil, errors.New("group name is required")
	}

	if currency == "" {
		currency = domain.DefaultCurrency
	} else if !domain.IsValidCurrency(currency) {
		return nil, errors.New("unknown currency")
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
		Currency:    currency,
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

// UpdateGroup changes the group's name and/or emoji, provided callerID is a
// member. A nil field is left unchanged, so a caller can patch just the name,
// just the emoji, or both. An empty (all-whitespace) name is rejected — every
// other field is otherwise unvalidated here, matching CreateGroup's emoji.
func (s *groupService) UpdateGroup(ctx context.Context, groupID, callerID uint, name, emoji *string) (*domain.Group, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, ErrGroupNotFound
	}
	if !isMember(group, callerID) {
		return nil, ErrNotGroupMember
	}
	if name != nil && strings.TrimSpace(*name) == "" {
		return nil, errors.New("group name is required")
	}

	if err := s.groupRepo.UpdateNameAndEmoji(ctx, groupID, name, emoji); err != nil {
		return nil, err
	}

	return s.groupRepo.GetByID(ctx, groupID)
}

// DeleteGroup checks that callerID created the group, then permanently
// deletes it and everything under it. Receipt images are collected before
// the DB delete and removed from storage afterward, best-effort: a failed
// image cleanup is logged and swallowed rather than undoing (or failing) a
// deletion that's already been confirmed to the caller.
func (s *groupService) DeleteGroup(ctx context.Context, groupID, callerID uint) error {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return ErrGroupNotFound
	}
	if group.CreatedBy != callerID {
		return ErrNotGroupCreator
	}

	paths, err := s.groupRepo.GetExpenseReceiptImagePaths(ctx, groupID)
	if err != nil {
		return err
	}

	if err := s.groupRepo.Delete(ctx, groupID); err != nil {
		return err
	}

	if s.storageService != nil {
		for _, path := range paths {
			if err := s.storageService.Delete(ctx, path); err != nil {
				slog.Error("failed to delete receipt image for a deleted group", "error", err, "path", path)
			}
		}
	}

	return nil
}
