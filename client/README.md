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

When the server has `auth.enabled = true`, every RPC call requires a valid JWT. Obtain a token from the HTTP login endpoint, then pass it to `WithBearerToken`:

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' \
  | jq -r '.token')
```

```go
client, err := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken(os.Getenv("QUEUETI_TOKEN")),
)
```

Tokens expire after 15 minutes. For long-running consumers, refresh the token before expiry and reconnect:

```go
// Refresh via the HTTP API
newToken := refreshToken(httpClient, oldToken)

// Reconnect with a fresh token
client.Close()
client, _ = queueti.Dial(addr, queueti.WithInsecure(), queueti.WithBearerToken(newToken))
consumer = client.NewConsumer(topic)
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
    "fmt"
    "log"
    "os"
    "os/signal"

    queueti "github.com/Joessst-Dev/queue-ti/client"
)

func main() {
    client, err := queueti.Dial("localhost:50051",
        queueti.WithInsecure(),
        queueti.WithBearerToken(os.Getenv("QUEUETI_TOKEN")),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Publish one message.
    producer := client.NewProducer()
    id, err := producer.Publish(context.Background(), "orders", []byte(`{"item":"book"}`))
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
        return nil
    }); err != nil {
        log.Fatal(err)
    }
}
```
