package service

import (
	"context"
	"errors"
	"testing"

	"spliteasy/internal/domain"
)

type fakeUserRepoForGroups struct {
	user *domain.User
}

func (f *fakeUserRepoForGroups) Create(_ context.Context, user *domain.User) error { return nil }
func (f *fakeUserRepoForGroups) Update(_ context.Context, user *domain.User) error { return nil }
func (f *fakeUserRepoForGroups) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	return f.user, nil
}
func (f *fakeUserRepoForGroups) GetByID(_ context.Context, id uint) (*domain.User, error) {
	if f.user == nil {
		return nil, errExpected
	}
	return f.user, nil
}
func (f *fakeUserRepoForGroups) UpdatePushPreferences(_ context.Context, _ uint, _, _, _, _ bool) error {
	return nil
}

var errExpected = errString("not found")

type errString string

func (e errString) Error() string { return string(e) }

func newGroupService(group *domain.Group) (*groupService, *fakeGroupRepo) {
	groupRepo := &fakeGroupRepo{group: group}
	svc := &groupService{
		groupRepo: groupRepo,
		userRepo:  &fakeUserRepoForGroups{user: &domain.User{ID: 1, Name: "Alice"}},
	}
	return svc, groupRepo
}

func newGroupServiceWithStorage(group *domain.Group, storage StorageService) (*groupService, *fakeGroupRepo) {
	svc, groupRepo := newGroupService(group)
	svc.storageService = storage
	return svc, groupRepo
}

func TestCreateGroup_GeneratesInviteToken(t *testing.T) {
	svc, _ := newGroupService(nil)

	group, err := svc.CreateGroup(context.Background(), "Asado", "🏔️", "", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.InviteToken == "" {
		t.Error("expected a generated invite token, got empty")
	}
}

func TestCreateGroup_DefaultsCurrencyToDefault(t *testing.T) {
	svc, _ := newGroupService(nil)

	group, err := svc.CreateGroup(context.Background(), "Asado", "🏔️", "", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.Currency != domain.DefaultCurrency {
		t.Errorf("expected default currency %q, got %q", domain.DefaultCurrency, group.Currency)
	}
}

func TestCreateGroup_PersistsCurrency(t *testing.T) {
	svc, _ := newGroupService(nil)

	group, err := svc.CreateGroup(context.Background(), "Asado", "🏔️", "ARS", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.Currency != "ARS" {
		t.Errorf("expected currency %q, got %q", "ARS", group.Currency)
	}
}

func TestCreateGroup_RejectsUnknownCurrency(t *testing.T) {
	svc, _ := newGroupService(nil)

	if _, err := svc.CreateGroup(context.Background(), "Asado", "🏔️", "GBP", 1); err == nil {
		t.Error("expected error for unknown currency")
	}
}

func TestGetInviteToken_RejectsNonMember(t *testing.T) {
	group := &domain.Group{ID: 1, InviteToken: "tok", Members: []domain.User{{ID: 1}}}
	svc, _ := newGroupService(group)

	if _, err := svc.GetInviteToken(context.Background(), 1, 99); err == nil {
		t.Error("expected error when a non-member requests the invite")
	}
}

func TestGetInviteToken_ReturnsTokenForMember(t *testing.T) {
	group := &domain.Group{ID: 1, InviteToken: "tok-123", Members: []domain.User{{ID: 7}}}
	svc, _ := newGroupService(group)

	token, err := svc.GetInviteToken(context.Background(), 1, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-123" {
		t.Errorf("expected existing token, got %q", token)
	}
}

func TestGetInviteToken_LazilyGeneratesWhenEmpty(t *testing.T) {
	group := &domain.Group{ID: 5, InviteToken: "", Members: []domain.User{{ID: 7}}}
	svc, repo := newGroupService(group)

	token, err := svc.GetInviteToken(context.Background(), 5, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected a lazily generated token")
	}
	if repo.updatedTokens[5] != token {
		t.Errorf("expected token to be persisted for group 5, got %q", repo.updatedTokens[5])
	}
}

func TestGetInviteToken_IsIdempotentOnceGenerated(t *testing.T) {
	group := &domain.Group{ID: 5, InviteToken: "", Members: []domain.User{{ID: 7}}}
	svc, _ := newGroupService(group)

	first, err := svc.GetInviteToken(context.Background(), 5, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := svc.GetInviteToken(context.Background(), 5, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Errorf("expected the same persisted token on repeat, got %q then %q", first, second)
	}
}

func TestJoinGroup_AddsMember(t *testing.T) {
	group := &domain.Group{ID: 3, InviteToken: "valid-token"}
	svc, repo := newGroupService(group)

	joined, err := svc.JoinGroup(context.Background(), "valid-token", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if joined.ID != 3 {
		t.Errorf("expected to join group 3, got %d", joined.ID)
	}
	if len(repo.addedMembers) != 1 || repo.addedMembers[0] != [2]uint{3, 42} {
		t.Errorf("expected user 42 added to group 3, got %+v", repo.addedMembers)
	}
}

func TestJoinGroup_RejectsEmptyToken(t *testing.T) {
	svc, _ := newGroupService(&domain.Group{ID: 1})

	if _, err := svc.JoinGroup(context.Background(), "", 1); err == nil {
		t.Error("expected error for empty token")
	}
}

func TestJoinGroup_RejectsInvalidToken(t *testing.T) {
	svc, _ := newGroupService(nil) // GetByInviteToken returns error when group is nil

	if _, err := svc.JoinGroup(context.Background(), "bogus", 1); err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestUpdateGroup_AllowsAnyMember(t *testing.T) {
	group := &domain.Group{ID: 1, Name: "Old Name", Emoji: "🏔️", CreatedBy: 7, Members: []domain.User{{ID: 7}, {ID: 42}}}
	svc, _ := newGroupService(group)

	updated, err := svc.UpdateGroup(context.Background(), 1, 42, strPtr("New Name"), strPtr("🎉"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "New Name" || updated.Emoji != "🎉" {
		t.Errorf("expected name/emoji updated, got %+v", updated)
	}
}

func TestUpdateGroup_PartialUpdateLeavesOtherFieldUnchanged(t *testing.T) {
	group := &domain.Group{ID: 1, Name: "Old Name", Emoji: "🏔️", Members: []domain.User{{ID: 7}}}
	svc, _ := newGroupService(group)

	updated, err := svc.UpdateGroup(context.Background(), 1, 7, strPtr("New Name"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("expected name updated, got %q", updated.Name)
	}
	if updated.Emoji != "🏔️" {
		t.Errorf("expected emoji left untouched, got %q", updated.Emoji)
	}
}

func TestUpdateGroup_RejectsNonMember(t *testing.T) {
	group := &domain.Group{ID: 1, Members: []domain.User{{ID: 7}}}
	svc, _ := newGroupService(group)

	_, err := svc.UpdateGroup(context.Background(), 1, 99, strPtr("New Name"), nil)
	if !errors.Is(err, ErrNotGroupMember) {
		t.Fatalf("expected ErrNotGroupMember, got %v", err)
	}
}

func TestUpdateGroup_RejectsEmptyName(t *testing.T) {
	group := &domain.Group{ID: 1, Name: "Old Name", Members: []domain.User{{ID: 7}}}
	svc, _ := newGroupService(group)

	_, err := svc.UpdateGroup(context.Background(), 1, 7, strPtr("   "), nil)
	if err == nil {
		t.Error("expected error for blank name")
	}
}

func TestUpdateGroup_GroupNotFound(t *testing.T) {
	svc, _ := newGroupService(nil)

	_, err := svc.UpdateGroup(context.Background(), 1, 7, strPtr("New Name"), nil)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func strPtr(s string) *string { return &s }

func TestDeleteGroup_AllowsCreator(t *testing.T) {
	group := &domain.Group{ID: 1, CreatedBy: 7}
	svc, repo := newGroupService(group)

	if err := svc.DeleteGroup(context.Background(), 1, 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deletedGroupID != 1 {
		t.Errorf("expected group 1 to be deleted, got deletedGroupID=%d", repo.deletedGroupID)
	}
}

func TestDeleteGroup_RejectsNonCreator(t *testing.T) {
	group := &domain.Group{ID: 1, CreatedBy: 7}
	svc, repo := newGroupService(group)

	err := svc.DeleteGroup(context.Background(), 1, 99)
	if !errors.Is(err, ErrNotGroupCreator) {
		t.Fatalf("expected ErrNotGroupCreator, got %v", err)
	}
	if repo.deletedGroupID != 0 {
		t.Error("expected the group to NOT be deleted when the caller isn't the creator")
	}
}

func TestDeleteGroup_GroupNotFound(t *testing.T) {
	svc, _ := newGroupService(nil)

	err := svc.DeleteGroup(context.Background(), 1, 7)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestDeleteGroup_DeletesReceiptImagesFromStorage(t *testing.T) {
	group := &domain.Group{ID: 1, CreatedBy: 7}
	storage := &fakeStorageService{}
	svc, repo := newGroupServiceWithStorage(group, storage)
	repo.receiptImagePaths = []string{"a.jpg", "b.jpg"}

	if err := svc.DeleteGroup(context.Background(), 1, 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(storage.deletedPaths) != 2 {
		t.Fatalf("expected 2 images deleted from storage, got %d: %+v", len(storage.deletedPaths), storage.deletedPaths)
	}
}

func TestDeleteGroup_StorageFailureDoesNotFailTheDelete(t *testing.T) {
	// The DB delete has already been confirmed by the time images are
	// cleaned up — a storage error must not turn a successful delete into a
	// failure response.
	group := &domain.Group{ID: 1, CreatedBy: 7}
	storage := &fakeStorageService{deleteErr: errors.New("network error")}
	svc, repo := newGroupServiceWithStorage(group, storage)
	repo.receiptImagePaths = []string{"a.jpg"}

	if err := svc.DeleteGroup(context.Background(), 1, 7); err != nil {
		t.Fatalf("expected DeleteGroup to succeed despite the storage error, got: %v", err)
	}
	if repo.deletedGroupID != 1 {
		t.Error("expected the group to still be deleted")
	}
}

func TestDeleteGroup_SkipsStorageCleanupWhenNotConfigured(t *testing.T) {
	group := &domain.Group{ID: 1, CreatedBy: 7}
	svc, repo := newGroupService(group) // storageService left nil, as in production when unconfigured
	repo.receiptImagePaths = []string{"a.jpg"}

	if err := svc.DeleteGroup(context.Background(), 1, 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deletedGroupID != 1 {
		t.Error("expected the group to still be deleted")
	}
}
