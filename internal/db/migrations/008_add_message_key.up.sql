ALTER TABLE messages ADD COLUMN key TEXT;

CREATE UNIQUE INDEX idx_messages_topic_key
    ON messages (topic, key)
    WHERE key IS NOT NULL AND status = 'pending';
