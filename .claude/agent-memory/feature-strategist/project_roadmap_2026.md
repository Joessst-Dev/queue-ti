---
name: Q2 2026 Feature Roadmap
description: Four features planned for implementation as of 2026-04-24: Retries, TTL, Avro schema validation, DLQ
type: project
---

As of 2026-04-24, the team plans to implement four features in this order (dependency-driven):

1. **Retries** — `QUEUETI_QUEUE_MAX_RETRIES` env var; retry_count + max_retries columns on messages table; Nack RPC
2. **Message TTL** — `QUEUETI_QUEUE_MESSAGE_TTL` env var; expires_at column on messages table; background reaper goroutine
3. **Dead-Letter Queue (DLQ)** — `QUEUETI_QUEUE_DLQ_THRESHOLD` env var; depends on Retries being done first; moves exhausted messages to `<topic>.dlq` topic
4. **Avro Schema Validation** — `QUEUETI_QUEUE_SCHEMA_REGISTRY_*` env vars; new `topic_schemas` table; validation at Enqueue time

**Why this order:** DLQ depends on retry_count existing (Feature 1). Avro is independent but highest effort; defer until reliability features are solid.

**How to apply:** When suggesting implementation tasks or scoping new work, assume these four are in-flight and deprioritize features that conflict with the messages table schema changes they introduce.
