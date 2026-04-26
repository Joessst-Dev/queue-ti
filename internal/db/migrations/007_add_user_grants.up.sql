CREATE TABLE IF NOT EXISTS user_grants (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action        TEXT        NOT NULL CHECK (action IN ('read', 'write', 'admin')),
    topic_pattern TEXT        NOT NULL DEFAULT '*',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_grants_user_id ON user_grants (user_id);
