ALTER TABLE messages
  ADD COLUMN dlq_threshold  INT,
  ADD COLUMN dlq_moved_at   TIMESTAMPTZ,
  ADD COLUMN original_topic TEXT;

CREATE INDEX idx_messages_failed
  ON messages (topic, status, updated_at)
  WHERE status = 'failed';
