package cache

import (
	"context"
	"time"
)

// Storage is the narrow subset of fiber.Storage this package needs.
// Defining it here avoids importing the fiber package into the cache layer.
// Note: fiber's Storage.Delete accepts a single key, not a variadic slice.
type Storage interface {
	Get(key string) ([]byte, error)
	Set(key string, val []byte, exp time.Duration) error
	Delete(key string) error
}

// Redis wraps a fiber-compatible storage backend as a Cache.
type Redis struct{ store Storage }

// NewRedis returns a Cache backed by the provided Storage implementation.
// In production this is a *redisstorage.Storage; in tests any Storage works.
func NewRedis(s Storage) *Redis { return &Redis{store: s} }

func (r *Redis) Get(_ context.Context, key string) ([]byte, error) {
	return r.store.Get(key)
}

func (r *Redis) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	return r.store.Set(key, val, ttl)
}

// Delete deletes each key individually to match the fiber Storage interface,
// which accepts a single key per call rather than a variadic slice.
func (r *Redis) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		if err := r.store.Delete(key); err != nil {
			return err
		}
	}
	return nil
}
