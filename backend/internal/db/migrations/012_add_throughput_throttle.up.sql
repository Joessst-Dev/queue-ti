ALTER TABLE topic_config
    ADD COLUMN IF NOT EXISTS throughput_limit INT;

CREATE TABLE IF NOT EXISTS topic_throughput (
    topic       TEXT        PRIMARY KEY,
    tokens      FLOAT       NOT NULL DEFAULT 0,
    last_refill TIMESTAMPTZ NOT NULL DEFAULT now()
);
