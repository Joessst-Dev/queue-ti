ALTER TABLE messages
  ADD COLUMN retry_count  INT         NOT NULL DEFAULT 0,
  ADD COLUMN max_retries  INT         NOT NULL DEFAULT 3,
  ADD COLUMN last_error   TEXT,
  ADD COLUMN expires_at   TIMESTAMPTZ;

CREATE INDEX idx_messages_exhausted
  ON messages (topic, retry_count, max_retries)
  WHERE status = 'pending';
