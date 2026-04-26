---
name: gRPC JWT auth and per-operation grant enforcement
description: gRPC interceptor uses JWT Bearer tokens (not Basic auth); GRPCServer holds userStore for per-operation grant checks
type: project
---

The gRPC `UnaryInterceptor` validates `Bearer <token>` via `users.ParseToken`, stores `*users.Claims` in context under an unexported `claimsKey{}` type, and exposes `auth.ClaimsFromContext(ctx)` for handlers to read.

`GRPCServer` accepts a `*users.Store` (nil = auth disabled). The `checkGrant` helper skips enforcement when `userStore` is nil or claims are absent. Admins bypass grant checks. Enqueue/Dequeue check `"write"` on the topic directly; Ack/Nack first call `queueService.TopicForMessage` to resolve the topic before checking grants.

**Why:** Replaced basic auth to align gRPC auth with the existing HTTP JWT flow. Grant enforcement means non-admin tokens are scoped to specific topics/actions stored in `user_grants`.

**How to apply:** When adding new gRPC handlers, always call `s.checkGrant(ctx, action, topic)` after input validation. For operations where the topic isn't in the request (like Ack/Nack), call `TopicForMessage` first and return `codes.NotFound` if it returns `ErrNotFound`.
