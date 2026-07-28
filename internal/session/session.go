// Package session implements a Redis-backed store for authenticated
// session API keys, issued by POST /api/auth/login and carrying admin
// rights for the duration of their TTL.
package session

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	keyPrefix = "session:"
	// TTL is how long a session API key remains valid after login.
	TTL = 24 * time.Hour
)

// Store is a Redis-backed store for session API keys.
type Store struct {
	redis *redis.Client
}

// NewStore wraps redisClient as a session Store.
func NewStore(redisClient *redis.Client) *Store {
	return &Store{redis: redisClient}
}

// Create generates a new session API key, stores it in Redis with a TTL of
// TTL, and returns it.
func (s *Store) Create(ctx context.Context) (string, error) {
	apiKey := uuid.NewString()
	if err := s.redis.Set(ctx, keyPrefix+apiKey, "admin", TTL).Err(); err != nil {
		return "", err
	}
	return apiKey, nil
}

// Valid reports whether apiKey is a currently-valid, unexpired session key.
func (s *Store) Valid(ctx context.Context, apiKey string) bool {
	if apiKey == "" {
		return false
	}
	n, err := s.redis.Exists(ctx, keyPrefix+apiKey).Result()
	return err == nil && n > 0
}

// Delete removes apiKey from the store. Deleting a key that isn't a
// currently-valid session (e.g. a static config-based key) is a harmless
// no-op.
func (s *Store) Delete(ctx context.Context, apiKey string) error {
	return s.redis.Del(ctx, keyPrefix+apiKey).Err()
}
