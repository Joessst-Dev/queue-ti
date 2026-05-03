ALTER TABLE user_grants
    DROP CONSTRAINT IF EXISTS user_grants_action_check,
    ADD  CONSTRAINT user_grants_action_check
         CHECK (action IN ('read', 'write', 'admin', 'consume'));

ALTER TABLE user_grants
    ADD COLUMN IF NOT EXISTS consumer_group TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_grants_consume
    ON user_grants (user_id, topic_pattern, consumer_group)
    WHERE action = 'consume';
