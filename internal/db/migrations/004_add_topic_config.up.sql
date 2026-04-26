CREATE TABLE IF NOT EXISTS topic_config (
    topic               TEXT        PRIMARY KEY,
    max_retries         INT,
    message_ttl_seconds INT,
    max_depth           INT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
