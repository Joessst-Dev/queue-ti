DROP INDEX IF EXISTS idx_messages_exhausted;
ALTER TABLE messages
  DROP COLUMN IF EXISTS expires_at,
  DROP COLUMN IF EXISTS last_error,
  DROP COLUMN IF EXISTS max_retries,
  DROP COLUMN IF EXISTS retry_count;
