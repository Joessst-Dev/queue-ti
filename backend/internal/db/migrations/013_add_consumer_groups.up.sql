CREATE TABLE IF NOT EXISTS consumer_groups (
    topic          TEXT        NOT NULL,
    consumer_group TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (topic, consumer_group)
);

CREATE TABLE IF NOT EXISTS message_deliveries (
    message_id         UUID        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    consumer_group     TEXT        NOT NULL,
    status             TEXT        NOT NULL DEFAULT 'pending',
    visibility_timeout TIMESTAMPTZ,
    retry_count        INT         NOT NULL DEFAULT 0,
    max_retries        INT         NOT NULL DEFAULT 0,
    last_error         TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, consumer_group)
);

CREATE INDEX IF NOT EXISTS idx_message_deliveries_dequeue
    ON message_deliveries (consumer_group, status, visibility_timeout, created_at)
    WHERE status = 'pending';

CREATE OR REPLACE FUNCTION fn_mint_deliveries()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO message_deliveries (message_id, consumer_group)
    SELECT NEW.id, cg.consumer_group
    FROM consumer_groups cg
    WHERE cg.topic = NEW.topic
    ON CONFLICT DO NOTHING;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_mint_deliveries
    AFTER INSERT ON messages
    FOR EACH ROW
    EXECUTE FUNCTION fn_mint_deliveries();
