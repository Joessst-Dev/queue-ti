---
name: bufconn testing pattern for gRPC
description: How to wire bufconn in-process gRPC servers for unit tests without a real network
type: feedback
---

Use `google.golang.org/grpc/test/bufconn` for all gRPC unit tests. The pattern is:

1. Create `bufconn.Listen(1024*1024)`, register the fake server, call `srv.Serve(lis)` in a goroutine.
2. Dial with `grpc.DialContext` + `grpc.WithContextDialer(func(...) { return lis.DialContext(ctx) })` + insecure credentials.
3. Expose a `DialConn(*grpc.ClientConn) (*Client, error)` constructor on the public client type so tests can inject the bufconn connection without going through `Dial`.
4. Fake servers embed `pb.UnimplementedQueueServiceServer` and override only the RPCs under test.
5. Use a `sync.Once`-closed channel (`streamReady`) in streaming fakes to let tests synchronise on "stream is open" before cancelling ctx.

**Why:** No real network or DB needed; tests are fast and hermetic. The `DialConn` escape hatch avoids duplicating the full option-parsing path while keeping it unexported from the package API perspective.

**How to apply:** Any time a new gRPC handler or consumer feature needs testing, reach for bufconn + a hand-rolled fake — not gomock or a real server.
