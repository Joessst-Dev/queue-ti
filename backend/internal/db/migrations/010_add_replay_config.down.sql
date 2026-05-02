ALTER TABLE topic_config
  DROP COLUMN IF EXISTS replayable,
  DROP COLUMN IF EXISTS replay_window_seconds;
