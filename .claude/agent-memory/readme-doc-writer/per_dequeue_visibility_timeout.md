---
name: Per-Dequeue Visibility Timeout Override Feature
description: Optional visibility_timeout_seconds field in DequeueRequest allows clients to override server default on a per-call basis
type: project
---

**Feature**: Per-dequeue configurable visibility timeout

**What was implemented**:
DequeueRequest proto message (field 2) now accepts an optional `uint32 visibility_timeout_seconds` field. When set to a value > 0, it overrides the server-wide `visibility_timeout` configuration for that single dequeue call. When omitted or nil, the server default applies unchanged.

**Validation behavior** (implemented in `internal/server/grpc.go`):
- If `visibility_timeout_seconds` is not set/nil: server default is used
- If `visibility_timeout_seconds > 0`: overrides server default
- If `visibility_timeout_seconds == 0`: rejected with `codes.InvalidArgument("visibility_timeout_seconds must be greater than zero")`

**Service layer** (`internal/queue/service.go`, Dequeue method):
- Signature: `Dequeue(ctx context.Context, topic string, visibilityTimeout time.Duration) (*Message, error)`
- If `visibilityTimeout > 0`, uses that; otherwise uses `s.visibilityTimeout` (the server default)

**Use case**: Slow consumers (e.g., batch processors) can request a longer timeout for individual calls without changing the global configuration, which might be lower (e.g., 30s) for fast consumers on the same topic.

**Documentation updates**:
1. Queue Mechanics section: Added explanation of per-dequeue override behavior
2. Dequeue API Reference: Updated proto definition to show optional field; added "Visibility Timeout Behavior" subsection with examples

**Why**: Flexibility in visibility timeout allows different consumer patterns on the same topic queue without operational reconfiguration.
