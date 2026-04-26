DROP INDEX IF EXISTS idx_messages_failed;
ALTER TABLE messages
  DROP COLUMN IF EXISTS original_topic,
  DROP COLUMN IF EXISTS dlq_moved_at,
  DROP COLUMN IF EXISTS dlq_threshold;
