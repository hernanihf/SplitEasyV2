package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RefreshTokenStore issues and consumes single-use refresh tokens, used to
// mint a new short-lived access token without forcing the user through the
// full Google OAuth flow again.
type RefreshTokenStore interface {
	// Create issues a new refresh token for userID, valid for ttl.
	Create(ctx context.Context, userID uint, ttl time.Duration) (token string, err error)
	// Consume validates and invalidates a refresh token in one step — so a
	// stolen, replayed token can be used at most once — returning the user
	// it belonged to. ok is false for a missing, already-used, or expired
	// token (indistinguishable on purpose: nothing downstream needs to tell
	// those apart).
	Consume(ctx context.Context, token string) (userID uint, ok bool, err error)
	// Revoke invalidates a refresh token immediately (logout). Revoking an
	// already-invalid token is not an error.
	Revoke(ctx context.Context, token string) error
}

func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

const refreshKeyPrefix = "refresh:"

// redisRefreshTokenStore persists tokens in Redis so they survive restarts
// and are revocable from any instance of the API.
type redisRefreshTokenStore struct {
	rdb *redis.Client
}

func NewRedisRefreshTokenStore(rdb *redis.Client) RefreshTokenStore {
	return &redisRefreshTokenStore{rdb}
}

func (s *redisRefreshTokenStore) Create(ctx context.Context, userID uint, ttl time.Duration) (string, error) {
	token, err := generateRefreshToken()
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, refreshKeyPrefix+token, userID, ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *redisRefreshTokenStore) Consume(ctx context.Context, token string) (uint, bool, error) {
	// GETDEL is atomic, so two concurrent refreshes with the same token can't
	// both succeed (only one will see the value; the other gets redis.Nil).
	val, err := s.rdb.GetDel(ctx, refreshKeyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	id, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return uint(id), true, nil
}

func (s *redisRefreshTokenStore) Revoke(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, refreshKeyPrefix+token).Err()
}

// inMemoryRefreshTokenStore is the local-dev fallback when REDIS_URL isn't
// set. Like the in-memory rate limiter, state resets on every restart.
type inMemoryRefreshTokenStore struct {
	mu     sync.Mutex
	tokens map[string]inMemoryRefreshEntry
}

type inMemoryRefreshEntry struct {
	userID  uint
	expires time.Time
}

func NewInMemoryRefreshTokenStore() RefreshTokenStore {
	return &inMemoryRefreshTokenStore{tokens: make(map[string]inMemoryRefreshEntry)}
}

func (s *inMemoryRefreshTokenStore) Create(_ context.Context, userID uint, ttl time.Duration) (string, error) {
	token, err := generateRefreshToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.tokens[token] = inMemoryRefreshEntry{userID: userID, expires: time.Now().Add(ttl)}
	s.mu.Unlock()
	return token, nil
}

func (s *inMemoryRefreshTokenStore) Consume(_ context.Context, token string) (uint, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.tokens[token]
	delete(s.tokens, token) // single-use regardless of outcome
	if !ok || time.Now().After(entry.expires) {
		return 0, false, nil
	}
	return entry.userID, true, nil
}

func (s *inMemoryRefreshTokenStore) Revoke(_ context.Context, token string) error {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
	return nil
}
