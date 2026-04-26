---
name: Log Level Configuration Documentation
description: Documentation for log_level config key with debug/info/warn/error levels, env var QUEUETI_LOG_LEVEL
type: project
---

Log level feature implemented as top-level `log_level` config key (not nested under queue, auth, etc.).

**Configuration Details:**
- **Config key**: `log_level` (top-level in YAML)
- **Env var**: `QUEUETI_LOG_LEVEL`
- **Default value**: `"info"`
- **Accepted values**: `debug`, `info`, `warn`, `error` (case-insensitive, parsed via `slog.Level.UnmarshalText`)
- **Startup behavior**: Resolved level is printed in the `"config loaded"` log line

**Semantics:**
- **DEBUG**: Per-message operations (enqueue, dequeue, ack, nack-retry) and HTTP requests — high volume, useful for local development and tracing individual messages
- **INFO**: Server startup, DLQ promotions, requeue, expiry reaper results, auth enabled notice — default production level
- **WARN**: Auth failures, DLQ threshold misconfiguration
- **ERROR**: Unexpected DB failures, server errors

**Documentation location**: README.md, Configuration section (lines 114, 138, 140-161):
- Line 114: Added to config.yaml example
- Line 138: Added to environment variables table
- Lines 140-161: New "Log Levels" subsection with level semantics and usage examples
