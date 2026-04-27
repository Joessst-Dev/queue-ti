# queue-ti Go Client Library

A Go client library for producing and consuming messages from a [queue-ti](https://github.com/Joessst-Dev/queue-ti) server.

## Installation

```bash
go get github.com/Joessst-Dev/queue-ti/client
```

Import as:

```go
import queueti "github.com/Joessst-Dev/queue-ti/client"
```

## Quick Start

### Producer

```go
client, err := queueti.Dial("localhost:50051", queueti.WithInsecure())
if err != nil {
    log.Fatal(err)
}
defer client.Close()

producer := client.NewProducer()

id, err := producer.Publish(ctx, "orders", []byte(`{"amount": 99.99}`),
    queueti.WithMetadata(map[string]string{"source": "checkout"}),
)
if err != nil {
    log.Fatal(err)
}
fmt.Println("enqueued:", id)
```

### Consumer

```go
client, err := queueti.Dial("localhost:50051", queueti.WithInsecure())
if err != nil {
    log.Fatal(err)
}
defer client.Close()

consumer := client.NewConsumer("orders",
    queueti.WithConcurrency(4),
    queueti.WithVisibilityTimeout(30),
)

ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()

err = consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    fmt.Printf("received: %s\n", msg.Payload)
    return nil // return nil to Ack; return error to Nack
})
```

`Consume` blocks until the context is cancelled. When your process receives `SIGINT` or `SIGTERM`, the context is cancelled and `Consume` returns `nil`.

---

## Connecting

### `queueti.Dial(addr string, opts ...dialOption) (*Client, error)`

Opens a gRPC connection to the server at `addr`.

```go
// No auth (local/dev)
client, err := queueti.Dial("localhost:50051", queueti.WithInsecure())

// With JWT authentication
client, err := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken("eyJ..."),
)
```

Always call `client.Close()` when done (typically via `defer`).

### Dial options

| Option | Description |
|---|---|
| `WithInsecure()` | Disable TLS — suitable for local and Docker deployments |
| `WithBearerToken(token string)` | Attach a JWT Bearer token to every RPC call |
| `WithTokenRefresher(TokenRefresher)` | Callback the library calls automatically to obtain a fresh token before expiry |
| `WithGRPCOption(grpc.DialOption)` | Pass a raw gRPC dial option for advanced configuration |

---

## Producing Messages

### `client.NewProducer() *Producer`

```go
producer := client.NewProducer()
```

### `producer.Publish(ctx, topic, payload, ...PublishOption) (string, error)`

Enqueues `payload` on `topic`. Returns the assigned message ID.

```go
id, err := producer.Publish(ctx, "payments", payload)

// With metadata
id, err := producer.Publish(ctx, "payments", payload,
    queueti.WithMetadata(map[string]string{
        "tenant": "acme",
        "trace":  traceID,
    }),
)
```

### Publish options

| Option | Description |
|---|---|
| `WithMetadata(map[string]string)` | Attach key-value metadata to the message |

---

## Consuming Messages

### `client.NewConsumer(topic string, ...ConsumerOption) *Consumer`

```go
consumer := client.NewConsumer("orders",
    queueti.WithConcurrency(8),
    queueti.WithVisibilityTimeout(60),
)
```

### Consumer options

| Option | Description |
|---|---|
| `WithConcurrency(n int)` | Number of messages processed concurrently (default: 1) |
| `WithVisibilityTimeout(seconds uint32)` | How long a message stays invisible while being processed (default: server setting, typically 30 s) |

### `consumer.Consume(ctx, HandlerFunc) error`

Starts consuming messages from the topic. Blocks until `ctx` is cancelled, then returns `nil`.

```go
err := consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    // Process the message.
    if err := process(msg.Payload); err != nil {
        return err // Nack — message re-queued after visibility timeout
    }
    return nil // Ack — message permanently removed
})
```

**Ack/Nack behaviour:**

| Handler return | Effect |
|---|---|
| `nil` | `Ack` — message is permanently removed from the queue |
| `error` | `Nack` — message re-appears after the visibility timeout; `error.Error()` is stored as the failure reason |

**Reconnection:** if the stream is interrupted (network error, server restart), `Consume` reconnects automatically with exponential backoff starting at 500 ms, doubling up to a maximum of 30 s.

**Panic recovery:** panics inside the handler are caught, treated as a `Nack`, and logged — they do not crash the consumer.

---

## The Message Type

```go
type Message struct {
    ID         string
    Topic      string
    Payload    []byte
    Metadata   map[string]string
    CreatedAt  time.Time
    RetryCount int
}
```

### `msg.Ack(ctx) error`

Permanently removes the message from the queue. When using `Consume` with a `HandlerFunc`, ack/nack is called automatically — call these directly only if you opted out of the handler model.

### `msg.Nack(ctx, reason string) error`

Returns the message to the queue. `reason` is stored as the failure string and is visible in the Admin UI.

---

## Authentication

When the server has `auth.enabled = true`, every RPC call requires a valid JWT. Tokens are issued by the server's HTTP API and expire after 15 minutes.

### Obtaining a token

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' \
  | jq -r '.token')
```

### Option 1 — Automatic refresh (recommended)

Pass an initial token and a `TokenRefresher` callback. The library decodes the JWT `exp` claim, sleeps until 60 seconds before expiry, and calls your callback to obtain a fresh token. The new token is applied to the next RPC call — no reconnection needed.

```go
func fetchToken(ctx context.Context) (string, error) {
    resp, err := http.PostForm("http://localhost:8080/api/auth/refresh",
        url.Values{"authorization": {currentToken}})
    // ... parse and return new token
}

client, err := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken(initialToken),  // used until first refresh
    queueti.WithTokenRefresher(fetchToken),
)
defer client.Close() // also stops the background refresh goroutine
```

If the refresher returns an error, the library retries with exponential backoff (5 s → 60 s) and logs each failure. RPCs will start failing with `Unauthenticated` once the token expires, so ensure the refresher can recover.

### Option 2 — Manual update

Call `client.SetToken` to swap the token on a live connection. The new token takes effect on the very next RPC call; no reconnection is needed.

```go
client, err := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken(initialToken),
)

// Later, when you have a fresh token:
if err := client.SetToken(newToken); err != nil {
    log.Fatal(err) // only errors if WithBearerToken was not used at Dial time
}
```

This is useful when token lifecycle is managed externally (e.g. a shared token store, a sidecar, or an existing refresh loop in your application).

### Option 3 — Static token (short-lived processes)

For scripts or batch jobs that complete within the 15-minute token window, a static token is sufficient:

```go
client, err := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken(os.Getenv("QUEUETI_TOKEN")),
)
```

---

## Error Handling

`Publish` wraps the underlying gRPC error and includes the topic name:

```go
id, err := producer.Publish(ctx, "orders", payload)
if err != nil {
    // e.g. "publish to topic \"orders\": rpc error: code = Unauthenticated ..."
    log.Fatal(err)
}
```

`Consume` only returns a non-nil error for programming mistakes (invalid configuration). Network errors and stream interruptions are handled internally via reconnection. A clean shutdown (context cancelled) always returns `nil`.

---

## Full Example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"

    queueti "github.com/Joessst-Dev/queue-ti/client"
)

func main() {
    initialToken := os.Getenv("QUEUETI_TOKEN")

    client, err := queueti.Dial("localhost:50051",
        queueti.WithInsecure(),
        queueti.WithBearerToken(initialToken),
        queueti.WithTokenRefresher(func(ctx context.Context) (string, error) {
            req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
                "http://localhost:8080/api/auth/refresh", nil)
            req.Header.Set("Authorization", "Bearer "+initialToken)
            resp, err := http.DefaultClient.Do(req)
            if err != nil {
                return "", err
            }
            defer resp.Body.Close()
            var body struct {
                Token string `json:"token"`
            }
            if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
                return "", err
            }
            return body.Token, nil
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Publish one message.
    producer := client.NewProducer()
    id, err := producer.Publish(context.Background(), "orders", []byte(`{"item":"book"}`),
        queueti.WithMetadata(map[string]string{"source": "example"}),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("published:", id)

    // Consume until SIGINT.
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    consumer := client.NewConsumer("orders", queueti.WithConcurrency(4))
    if err := consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
        fmt.Printf("[%s] %s\n", msg.ID, msg.Payload)
        return nil // nil = Ack
    }); err != nil {
        log.Fatal(err)
    }
}
```
