CREATE TABLE IF NOT EXISTS topic_schemas (
    topic              TEXT PRIMARY KEY,
    schema_json        TEXT NOT NULL,
    version            INT NOT NULL DEFAULT 1,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
