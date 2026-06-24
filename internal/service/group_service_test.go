package service

import (
	"testing"

	"spliteasy/internal/domain"
)

type fakeUserRepoForGroups struct {
	user *domain.User
}

func (f *fakeUserRepoForGroups) Create(user *domain.User) error { return nil }
func (f *fakeUserRepoForGroups) Update(user *domain.User) error { return nil }
func (f *fakeUserRepoForGroups) GetByEmail(email string) (*domain.User, error) {
	return f.user, nil
}
func (f *fakeUserRepoForGroups) GetByID(id uint) (*domain.User, error) {
	if f.user == nil {
		return nil, errExpected
	}
	return f.user, nil
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

func TestCreateGroup_GeneratesInviteToken(t *testing.T) {
	svc, _ := newGroupService(nil)

	group, err := svc.CreateGroup("Asado", "🏔️", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group.InviteToken == "" {
		t.Error("expected a generated invite token, got empty")
	}
}

func TestGetInviteToken_RejectsNonMember(t *testing.T) {
	group := &domain.Group{ID: 1, InviteToken: "tok", Members: []domain.User{{ID: 1}}}
	svc, _ := newGroupService(group)

	if _, err := svc.GetInviteToken(1, 99); err == nil {
		t.Error("expected error when a non-member requests the invite")
	}
}

func TestGetInviteToken_ReturnsTokenForMember(t *testing.T) {
	group := &domain.Group{ID: 1, InviteToken: "tok-123", Members: []domain.User{{ID: 7}}}
	svc, _ := newGroupService(group)

	token, err := svc.GetInviteToken(1, 7)
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

	token, err := svc.GetInviteToken(5, 7)
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

func TestJoinGroup_AddsMember(t *testing.T) {
	group := &domain.Group{ID: 3, InviteToken: "valid-token"}
	svc, repo := newGroupService(group)

	joined, err := svc.JoinGroup("valid-token", 42)
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

	if _, err := svc.JoinGroup("", 1); err == nil {
		t.Error("expected error for empty token")
	}
}

func TestJoinGroup_RejectsInvalidToken(t *testing.T) {
	svc, _ := newGroupService(nil) // GetByInviteToken returns error when group is nil

	if _, err := svc.JoinGroup("bogus", 1); err == nil {
		t.Error("expected error for invalid token")
	}
}
