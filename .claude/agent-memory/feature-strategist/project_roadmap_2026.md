---
name: Q2 2026 Feature Roadmap
description: Status of originally planned features as of 2026-04-26; all four now shipped except Avro schema validation
type: project
---

As of 2026-04-26, the four originally planned features are confirmed built:

1. **Retries** — DONE (retry_count, max_retries columns; Nack RPC)
2. **Message TTL** — DONE (expires_at column; background reaper)
3. **Dead-Letter Queue (DLQ)** — DONE (messages promoted after N nacks; requeue from admin UI)
4. **Avro Schema Validation** — NOT YET BUILT (highest effort; was deferred)

The team is now looking for the next three features beyond the original four.

**Why:** Avro schema validation is still on the table but the team wants fresh recommendations on what else to prioritize alongside or instead of it.

**How to apply:** When suggesting next features, treat Retries/TTL/DLQ as existing primitives to build on. Avro schema validation is a live candidate — score it honestly against alternatives.

## Next Three Features (recommended 2026-04-26)

1. **Configurable visibility timeout per dequeue** — S / Med. Proto field on DequeueRequest; one queue method change. Quick consumer unblocking win.
2. **Topic-level configuration** — M / High. New `topics` table, per-topic TTL/retries/max-depth, config UI tab. Highest leverage; enables multi-tenancy.
3. **Scheduled / delayed message delivery** — M / High. New `deliver_at` column on `messages`; filter in dequeue query. Unlocks backoff, deferred jobs, cron-style patterns.

Avro schema validation ranked 4th — high effort, niche audience, deferred again.
