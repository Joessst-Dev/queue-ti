package cache

import (
	"context"
	"time"
)

// Cache is a simple key-value byte cache with optional TTL.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

// Noop is a no-op Cache used when Redis is not configured.
// Every operation is a silent no-op, preserving pre-Redis behaviour.
type Noop struct{}

func (Noop) Get(_ context.Context, _ string) ([]byte, error)                  { return nil, nil }
func (Noop) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (Noop) Delete(_ context.Context, _ ...string) error                      { return nil }
