---
name: Topic Registration Feature Documentation
description: Documented the require_topic_registration config flag in the README
type: project
---

**Feature**: Topic registration enforcement flag for queue-ti

**Date Documented**: 2026-04-27

**What was added**:
- New config key `queue.require_topic_registration` (boolean, default `false`)
- Environment variable override: `QUEUETI_QUEUE_REQUIRE_TOPIC_REGISTRATION`
- Admin UI enhancement: "Add config" button now labelled "New Topic" when feature is enabled
- Empty-state copy in Topics section updated to guide topic registration

**Why**: When enabled, the enqueue endpoint rejects messages to topics that have no row in the `topic_config` table. This prevents silent failures from typos in topic names and is recommended for production deployments where topic names are fixed.

**How the feature works**:
- Default is `false` (no registration required, backward compatible)
- When `true`, enqueue to unregistered topics returns HTTP 422 / gRPC `FailedPrecondition`
- Topics are registered by creating a configuration entry via `PUT /api/topic-configs/:topic`
- The `topic_config` table doubles as the topic registry

**Documentation sections updated**:
1. Configuration File section: Added `require_topic_registration: false` to the queue config example
2. Environment Variables table: Added `QUEUETI_QUEUE_REQUIRE_TOPIC_REGISTRATION` entry
3. New subsection "Topic Registration" under Per-Topic Configuration: Explains the feature, use cases, behavior, and provides curl examples

**Key terminology used in docs**:
- "Topic registration" (not "topic allowlisting")
- "Registration is required" when the flag is enabled
- Error response: HTTP 422 (for HTTP) / gRPC `FailedPrecondition` (for gRPC)
- Registration happens via `PUT /api/topic-configs/:topic` (same endpoint used for config overrides)
