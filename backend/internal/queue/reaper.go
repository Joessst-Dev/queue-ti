package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

// tryWithReaperLock acquires a transaction-level PostgreSQL advisory lock for
// lockKey, then calls fn inside that transaction and commits. Returns nil when
// fn succeeds. Returns nil (without calling fn) when another instance already
// holds the lock — the caller should silently skip that tick.
func (s *Service) tryWithReaperLock(ctx context.Context, lockKey int64, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reaper lock begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var acquired bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockKey).Scan(&acquired); err != nil {
		return fmt.Errorf("reaper advisory lock: %w", err)
	}
	if !acquired {
		slog.Debug("reaper tick skipped: lock held by another instance", "lock_key", lockKey)
		return nil
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// StartExpiryReaper launches a background goroutine that periodically marks
// expired messages (expires_at < now()) as 'expired'. It runs until ctx is
// cancelled. The first tick fires immediately (after interval).
func (s *Service) StartExpiryReaper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := s.tryWithReaperLock(ctx, expiryReaperLockKey, func(tx pgx.Tx) error {
					tag, err := tx.Exec(ctx, `
						UPDATE messages
						SET    status     = 'expired',
						       updated_at = now()
						WHERE  expires_at IS NOT NULL
						  AND  expires_at < now()
						  AND  status IN ('pending', 'processing')
					`)
					if err != nil {
						return err
					}
					if n := tag.RowsAffected(); n > 0 {
						s.recorder.RecordExpired(n)
						slog.Info("expiry reaper expired messages", "count", n)
					}
					return nil
				})
				if err != nil {
					slog.Error("expiry reaper failed", "error", err)
				}
			}
		}
	}()
}

// RunExpiryReaperOnce marks all eligible expired messages as 'expired' in a
// single pass and returns the number of rows affected. It is the one-shot
// equivalent of the background StartExpiryReaper ticker.
func (s *Service) RunExpiryReaperOnce(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE messages
		SET    status     = 'expired',
		       updated_at = now()
		WHERE  expires_at IS NOT NULL
		  AND  expires_at < now()
		  AND  status IN ('pending', 'processing')
	`)
	if err != nil {
		return 0, fmt.Errorf("run expiry reaper: %w", err)
	}
	n := tag.RowsAffected()
	if n > 0 {
		s.recorder.RecordExpired(n)
		slog.Info("expiry reaper (manual) expired messages", "count", n)
	}
	return n, nil
}

const archiveReaperSQL = `
	DELETE FROM message_log ml
	USING topic_config tc
	WHERE ml.topic = tc.topic
	  AND tc.replayable = true
	  AND tc.replay_window_seconds IS NOT NULL
	  AND tc.replay_window_seconds > 0
	  AND ml.acked_at < now() - (tc.replay_window_seconds || ' seconds')::interval
`

// runDeleteWork deletes all expired messages and expired message_log rows
// inside an existing transaction. source ("manual" or "scheduled") is included
// in log output so that the two callers remain distinguishable in production.
// Returns the number of messages rows deleted.
func (s *Service) runDeleteWork(ctx context.Context, tx pgx.Tx, source string) (int64, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM messages WHERE status = 'expired'`)
	if err != nil {
		return 0, err
	}
	n := tag.RowsAffected()
	if n > 0 {
		s.recorder.RecordDeleted(n)
		slog.Info("delete reaper removed expired messages", "source", source, "count", n)
	}
	archiveTag, err := tx.Exec(ctx, archiveReaperSQL)
	if err != nil {
		return 0, err
	}
	if an := archiveTag.RowsAffected(); an > 0 {
		slog.Info("archive reaper removed expired message_log rows", "source", source, "count", an)
	}
	return n, nil
}

// RunDeleteReaperOnce permanently deletes all messages with status 'expired'
// and expired message_log rows in a single pass, returning the number of
// messages rows deleted.
func (s *Service) RunDeleteReaperOnce(ctx context.Context) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("run delete reaper begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	n, err := s.runDeleteWork(ctx, tx, "manual")
	if err != nil {
		return 0, fmt.Errorf("run delete reaper: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("run delete reaper commit: %w", err)
	}
	return n, nil
}

// RunArchiveReaperOnce deletes message_log rows that have aged past each
// topic's replay_window_seconds. Topics with replayable = false or no window
// configured are skipped. Returns the total number of rows deleted.
func (s *Service) RunArchiveReaperOnce(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, archiveReaperSQL)
	if err != nil {
		return 0, fmt.Errorf("run archive reaper: %w", err)
	}
	n := tag.RowsAffected()
	if n > 0 {
		slog.Info("archive reaper removed expired message_log rows", "count", n)
	}
	return n, nil
}

// StartDeleteReaper schedules the delete reaper using the given cron
// expression. If schedule is empty it is a no-op and returns a no-op stop
// function. The stop function stops the cron scheduler gracefully.
// The stop func is also stored internally so UpdateDeleteReaperSchedule can
// swap the cron at runtime.
func (s *Service) StartDeleteReaper(ctx context.Context, schedule string) (stop func(), err error) {
	if schedule == "" {
		noop := func() {}
		s.deleteReaperMu.Lock()
		s.deleteReaperStop = noop
		s.deleteReaperMu.Unlock()
		return noop, nil
	}

	c := cron.New()
	_, err = c.AddFunc(schedule, func() {
		runErr := s.tryWithReaperLock(ctx, deleteReaperLockKey, func(tx pgx.Tx) error {
			_, err := s.runDeleteWork(ctx, tx, "scheduled")
			return err
		})
		if runErr != nil {
			slog.Error("delete reaper failed", "error", runErr)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("start delete reaper: invalid schedule %q: %w", schedule, err)
	}

	c.Start()
	stopFn := func() { c.Stop() }
	s.deleteReaperMu.Lock()
	s.deleteReaperStop = stopFn
	s.deleteReaperMu.Unlock()
	return stopFn, nil
}

// GetDeleteReaperSchedule returns the cron schedule stored in system_settings.
// Returns "" if no schedule has been persisted.
func (s *Service) GetDeleteReaperSchedule(ctx context.Context) (string, error) {
	var schedule string
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = 'delete_reaper_schedule'`,
	).Scan(&schedule)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get delete reaper schedule: %w", err)
	}
	return schedule, nil
}

// UpdateDeleteReaperSchedule validates schedule, persists it to system_settings,
// stops the current cron, and starts a new one. An empty schedule disables the cron.
func (s *Service) UpdateDeleteReaperSchedule(ctx context.Context, schedule string) error {
	if schedule != "" {
		if _, parseErr := cron.ParseStandard(schedule); parseErr != nil {
			return fmt.Errorf("invalid cron schedule: %w", parseErr)
		}
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO system_settings (key, value) VALUES ('delete_reaper_schedule', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, schedule)
	if err != nil {
		return fmt.Errorf("persist delete reaper schedule: %w", err)
	}

	s.deleteReaperMu.Lock()
	if s.deleteReaperStop != nil {
		s.deleteReaperStop()
		s.deleteReaperStop = nil
	}
	s.deleteReaperMu.Unlock()

	if schedule == "" {
		return nil
	}
	_, err = s.StartDeleteReaper(ctx, schedule)
	return err
}
