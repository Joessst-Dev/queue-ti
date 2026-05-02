CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic TEXT NOT NULL,
    payload BYTEA NOT NULL,
    metadata JSONB,
    status TEXT NOT NULL DEFAULT 'pending',
    visibility_timeout TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_messages_dequeue
    ON messages (topic, status, visibility_timeout, created_at)
    WHERE status = 'pending';

