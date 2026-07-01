package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// storeFactories lets the same behavioral tests run against both backends,
// so they can't silently drift apart.
func storeFactories(t *testing.T) map[string]RefreshTokenStore {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	return map[string]RefreshTokenStore{
		"redis":     NewRedisRefreshTokenStore(rdb),
		"in-memory": NewInMemoryRefreshTokenStore(),
	}
}

func TestRefreshTokenStore_CreateThenConsume(t *testing.T) {
	for name, store := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			token, err := store.Create(context.Background(), 42, time.Hour)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			userID, ok, err := store.Consume(context.Background(), token)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Fatal("expected the freshly created token to be valid")
			}
			if userID != 42 {
				t.Fatalf("expected user id 42, got %d", userID)
			}
		})
	}
}

func TestRefreshTokenStore_ConsumeIsSingleUse(t *testing.T) {
	for name, store := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			token, err := store.Create(context.Background(), 1, time.Hour)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if _, ok, err := store.Consume(context.Background(), token); err != nil || !ok {
				t.Fatalf("first consume should succeed: ok=%v err=%v", ok, err)
			}
			if _, ok, err := store.Consume(context.Background(), token); err != nil || ok {
				t.Fatalf("second consume of the same token should fail: ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestRefreshTokenStore_ConsumeRejectsUnknownToken(t *testing.T) {
	for name, store := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			_, ok, err := store.Consume(context.Background(), "never-issued")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatal("expected an unknown token to be rejected")
			}
		})
	}
}

// Expiry needs a different time-advancement mechanism per backend: the
// in-memory store checks real wall-clock time, while miniredis needs
// FastForward (it doesn't poll the system clock on its own), so this can't
// share the same loop as the other tests.

func TestInMemoryRefreshTokenStore_ConsumeRejectsExpiredToken(t *testing.T) {
	store := NewInMemoryRefreshTokenStore()

	token, err := store.Create(context.Background(), 1, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if _, ok, err := store.Consume(context.Background(), token); err != nil || ok {
		t.Fatalf("expected an expired token to be rejected: ok=%v err=%v", ok, err)
	}
}

func TestRedisRefreshTokenStore_ConsumeRejectsExpiredToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewRedisRefreshTokenStore(rdb)

	token, err := store.Create(context.Background(), 1, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mr.FastForward(50 * time.Millisecond)

	if _, ok, err := store.Consume(context.Background(), token); err != nil || ok {
		t.Fatalf("expected an expired token to be rejected: ok=%v err=%v", ok, err)
	}
}

func TestRefreshTokenStore_RevokeInvalidatesTheToken(t *testing.T) {
	for name, store := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			token, err := store.Create(context.Background(), 1, time.Hour)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if err := store.Revoke(context.Background(), token); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if _, ok, err := store.Consume(context.Background(), token); err != nil || ok {
				t.Fatalf("expected the revoked token to be rejected: ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestRefreshTokenStore_RevokeUnknownTokenIsNotAnError(t *testing.T) {
	for name, store := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			if err := store.Revoke(context.Background(), "never-issued"); err != nil {
				t.Fatalf("revoking a nonexistent token should be a no-op, got: %v", err)
			}
		})
	}
}
