package users

import (
	"context"
	"errors"
	"log/slog"
)

func SeedAdminUser(ctx context.Context, store *Store, username, password string) error {
	existing, err := store.List(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	if username == "" || password == "" {
		slog.Warn("no users exist and auth.username/auth.password are empty: skipping admin seed")
		return nil
	}
	_, err = store.Create(ctx, username, password, true)
	if err != nil && !errors.Is(err, ErrDuplicate) {
		return err
	}
	slog.Info("seeded initial admin user", "username", username)
	return nil
}
