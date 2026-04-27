package queueti

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// tokenStore is a goroutine-safe container for a JWT string.
type tokenStore struct {
	mu    sync.RWMutex
	token string
}

func newTokenStore(initial string) *tokenStore {
	return &tokenStore{token: initial}
}

func (s *tokenStore) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func (s *tokenStore) set(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
}

// tokenExpiry decodes the JWT payload (middle segment) without verifying the
// signature and returns the exp claim as a time.Time.
// Returns zero time and a non-nil error if the token is malformed or has no exp.
func tokenExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("malformed JWT: expected 3 segments, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("unmarshal JWT payload: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("JWT has no exp claim")
	}
	return time.Unix(claims.Exp, 0), nil
}
