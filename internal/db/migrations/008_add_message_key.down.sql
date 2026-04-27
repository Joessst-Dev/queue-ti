DROP INDEX IF EXISTS idx_messages_topic_key;
ALTER TABLE messages DROP COLUMN IF EXISTS key;
