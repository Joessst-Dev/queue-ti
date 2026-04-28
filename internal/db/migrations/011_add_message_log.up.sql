CREATE TABLE IF NOT EXISTS message_log (
    id             UUID        NOT NULL,
    topic          TEXT        NOT NULL,
    key            TEXT,
    payload        BYTEA       NOT NULL,
    metadata       JSONB,
    retry_count    INT         NOT NULL DEFAULT 0,
    max_retries    INT         NOT NULL DEFAULT 3,
    last_error     TEXT,
    original_topic TEXT,
    created_at     TIMESTAMPTZ NOT NULL,
    acked_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (topic, acked_at, id)
);
CREATE INDEX IF NOT EXISTS idx_message_log_topic_acked ON message_log (topic, acked_at);
