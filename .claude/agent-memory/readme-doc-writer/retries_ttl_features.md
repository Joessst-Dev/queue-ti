---
name: Retries & TTL Features Added
description: Documentation updates for message retry count, max retries, TTL, and automatic expiry reaper
type: project
---

## Features Added (2026-04-24)

### Retry Mechanism
- Every message now has `retry_count` (integer, incremented on each Nack) and `max_retries` (set at enqueue, default 3)
- Consumers call `Nack(id, error)` to signal failure; if retries remain, message reverts to pending
- If retries exhausted, message status becomes `failed` (no longer dequeued)
- Error message stored in `last_error` field

**Configuration**:
- `QUEUETI_QUEUE_MAX_RETRIES=3` (default, can override per-message at enqueue)
- `queue.max_retries` in config.yaml

### Message TTL & Expiry Reaper
- New `expires_at` field on every message (set at enqueue based on `message_ttl`)
- Background goroutine (expiry reaper) runs every 60 seconds, marks messages with `expires_at < now()` as `expired`
- Expired messages cannot be dequeued (same as failed)

**Configuration**:
- `QUEUETI_QUEUE_MESSAGE_TTL=24h` (default, `0` = no expiry)
- `queue.message_ttl` in config.yaml

### New Message Statuses
- `pending` — Ready to dequeue (initial or reset after nack with retries)
- `processing` — Held by consumer
- `deleted` — Acknowledged
- `failed` — Nacked with no retries remaining
- `expired` — TTL elapsed and marked by reaper

### New RPC & HTTP Endpoints
- `Nack(NackRequest)` gRPC RPC
- `POST /api/messages/:id/nack` HTTP endpoint (optional JSON body: `{"error": "..."}`)

## Updated Sections in README
1. **Features** — Added retries, TTL, Nack
2. **Configuration table** — Added `QUEUETI_QUEUE_MAX_RETRIES` and `QUEUETI_QUEUE_MESSAGE_TTL`
3. **config.yaml example** — Added queue.max_retries and queue.message_ttl
4. **Queue Mechanics** — Expanded data model (new columns), message statuses, updated lifecycle
5. **gRPC API** — Added Nack RPC with protobuf examples
6. **HTTP Admin API** — Added POST /api/messages/:id/nack endpoint
7. **Known Limitations** — Removed "no message expiration", updated DLQ description (failed messages stay in table)
8. **New section: Automatic Expiry and Retry Management** — Explains reaper, retry behavior, configuration

## Key Source Files
- `internal/queue/service.go` — Nack(), StartExpiryReaper(), updated Enqueue/Dequeue
- `internal/config/config.go` — MaxRetries, MessageTTL in QueueConfig
- `proto/queue.proto` — NackRequest/NackResponse gRPC definitions
- `internal/server/http.go` — nackMessage HTTP handler

## Configuration Defaults
- visibility_timeout: 30s
- max_retries: 3
- message_ttl: 24h (can be 0 for no expiry)
