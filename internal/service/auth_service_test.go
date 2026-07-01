package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"spliteasy/internal/domain"
)

type fakeRefreshStore struct {
	tokens map[string]uint
	seq    int
}

func newFakeRefreshStore() *fakeRefreshStore {
	return &fakeRefreshStore{tokens: make(map[string]uint)}
}

func (f *fakeRefreshStore) Create(_ context.Context, userID uint, _ time.Duration) (string, error) {
	f.seq++
	token := fmt.Sprintf("token-%d-%d", userID, f.seq)
	f.tokens[token] = userID
	return token, nil
}

func (f *fakeRefreshStore) Consume(_ context.Context, token string) (uint, bool, error) {
	userID, ok := f.tokens[token]
	delete(f.tokens, token)
	return userID, ok, nil
}

func (f *fakeRefreshStore) Revoke(_ context.Context, token string) error {
	delete(f.tokens, token)
	return nil
}

type fakeUserRepoForAuth struct {
	usersByID map[uint]*domain.User
}

func (f *fakeUserRepoForAuth) Create(_ context.Context, _ *domain.User) error { return nil }
func (f *fakeUserRepoForAuth) Update(_ context.Context, _ *domain.User) error { return nil }

func (f *fakeUserRepoForAuth) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeUserRepoForAuth) GetByID(_ context.Context, id uint) (*domain.User, error) {
	u, ok := f.usersByID[id]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func newAuthServiceForTest() (*authService, *fakeRefreshStore) {
	refreshStore := newFakeRefreshStore()
	svc := &authService{
		userRepo: &fakeUserRepoForAuth{
			usersByID: map[uint]*domain.User{1: {ID: 1, Name: "Alice", Email: "alice@test.com"}},
		},
		refreshStore: refreshStore,
	}
	return svc, refreshStore
}

func TestRefreshToken_IssuesNewAccessAndRefreshTokens(t *testing.T) {
	svc, refreshStore := newAuthServiceForTest()
	original, err := refreshStore.Create(context.Background(), 1, refreshTokenTTL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	access, refresh, err := svc.RefreshToken(context.Background(), original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}
	if refresh == original {
		t.Error("expected the refresh token to rotate, got the same token back")
	}
}

func TestRefreshToken_RejectsUnknownToken(t *testing.T) {
	svc, _ := newAuthServiceForTest()

	if _, _, err := svc.RefreshToken(context.Background(), "not-a-real-token"); err == nil {
		t.Fatal("expected an error for an unknown refresh token")
	}
}

func TestRefreshToken_TokenIsSingleUse(t *testing.T) {
	svc, refreshStore := newAuthServiceForTest()
	original, err := refreshStore.Create(context.Background(), 1, refreshTokenTTL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, _, err := svc.RefreshToken(context.Background(), original); err != nil {
		t.Fatalf("first refresh should succeed: %v", err)
	}
	if _, _, err := svc.RefreshToken(context.Background(), original); err == nil {
		t.Fatal("expected the second refresh with the same (already-consumed) token to fail")
	}
}

func TestRefreshToken_RejectsTokenForUnknownUser(t *testing.T) {
	svc, refreshStore := newAuthServiceForTest()
	// A token for a user id that doesn't exist in the repo (e.g. deleted
	// between issuing the refresh token and using it).
	orphaned, err := refreshStore.Create(context.Background(), 999, refreshTokenTTL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, _, err := svc.RefreshToken(context.Background(), orphaned); err == nil {
		t.Fatal("expected an error when the refresh token's user no longer exists")
	}
}

func TestLogout_RevokesTheRefreshToken(t *testing.T) {
	svc, refreshStore := newAuthServiceForTest()
	token, err := refreshStore.Create(context.Background(), 1, refreshTokenTTL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := svc.Logout(context.Background(), token); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, _, err := svc.RefreshToken(context.Background(), token); err == nil {
		t.Fatal("expected the revoked refresh token to be rejected")
	}
}
