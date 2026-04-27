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

**grpc.NewClient + bufconn:** When the client library uses `grpc.NewClient` internally (not `grpc.DialContext`), use the address `"passthrough:///bufnet"` (not plain `"bufnet"`). The `passthrough:///` scheme tells the gRPC name resolver to skip DNS and hand the address straight to the context dialer — without it `grpc.NewClient` returns "produced zero addresses".

**Why:** No real network or DB needed; tests are fast and hermetic. The `DialConn` escape hatch avoids duplicating the full option-parsing path. The `passthrough:///` pattern is required because `grpc.NewClient` enforces name resolution by default, unlike the deprecated `grpc.DialContext`.

**How to apply:** Any time a new gRPC handler or consumer feature needs testing, reach for bufconn + a hand-rolled fake — not gomock or a real server. Use `grpc.DialContext` + `DialConn` when you don't need `PerRPCCredentials` on the conn; use `queueti.Dial("passthrough:///bufnet", ...)` when you do.
