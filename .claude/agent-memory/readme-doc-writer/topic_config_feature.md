---
name: Topic-Level Configuration Feature Documentation
description: Per-topic overrides for max_retries, message_ttl_seconds, max_depth with HTTP API and admin UI support
type: project
---

**Feature**: Per-topic configuration overrides

**What it provides**:
- `max_retries` — Override global retry limit per topic
- `message_ttl_seconds` — Override global TTL per topic (0 = no expiry)
- `max_depth` — Set queue capacity limit per topic (0/null = unlimited); `Enqueue` returns 429 when reached

**HTTP API endpoints** (all auth-protected):
- `GET /api/topic-configs` — list all configurations
- `PUT /api/topic-configs/:topic` — create/update (omit field or send null to revert to global)
- `DELETE /api/topic-configs/:topic` — delete (reverts all to global)

**Database**: `topic_config` table with columns: topic (PK), max_retries, message_ttl_seconds, max_depth, created_at, updated_at. A nil pointer field means "use global default".

**Handler validation**: Topic names ending in `.dlq` are rejected with HTTP 400 (reserved for dead-letter queues).

**Queue depth enforcement**: `Enqueue` checks max_depth and returns HTTP 429 with error "queue is at maximum depth for this topic" when limit is reached. Counted as pending + processing messages on the topic.

**Admin UI**: New "Config" tab provides inline-editable table for all topic configurations without server restart.

**Precedence rule**: Per-topic overrides take priority over global defaults (QUEUETI_QUEUE_* env vars or config.yaml).

**How to apply**: When documenting enqueue behavior, mention 429 capacity guard. When discussing configuration, highlight that per-topic settings can override globals without downtime.
