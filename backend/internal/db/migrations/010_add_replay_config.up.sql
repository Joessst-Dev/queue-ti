ALTER TABLE topic_config ADD COLUMN IF NOT EXISTS replayable            BOOL NOT NULL DEFAULT false;
ALTER TABLE topic_config ADD COLUMN IF NOT EXISTS replay_window_seconds INT;
