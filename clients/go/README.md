# queue-ti Go Client Library

A Go client library for producing and consuming messages from a [queue-ti](https://github.com/Joessst-Dev/queue-ti) server.

## Installation

```bash
go get github.com/Joessst-Dev/queue-ti/clients/go
```

Import as:

```go
import queueti "github.com/Joessst-Dev/queue-ti/clients/go"
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
| `WithKey(key string)` | Set a deduplication key for upsert semantics |

#### Idempotent publishing with `WithKey`

When you publish a message with a key, queue-ti uses upsert semantics: if a pending message with the same topic and key already exists in the queue, its payload and metadata are updated in place (the existing message ID is returned). This ensures idempotency when retrying publish operations.

**Caveat:** Once a message begins processing (transitions to `processing` status), it is no longer considered "pending". A key match only applies to messages awaiting processing. If the keyed message is already processing, a new row is inserted.

```go
// Idempotent publish: if a message with topic="orders" and key="order-123" 
// exists and is pending, it is updated. Otherwise a new message is created.
id, err := producer.Publish(ctx, "orders", []byte(`{"amount": 150.00}`),
    queueti.WithKey("order-123"),
    queueti.WithMetadata(map[string]string{"customer": "acme"}),
)
if err != nil {
    log.Fatal(err)
}
fmt.Println("message id:", id) // same on retry if order-123 is still pending
```

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
| `WithConsumerGroup(name string)` | Consumer group name for independent message consumption; see [Consumer Groups](#consumer-groups) |

#### Handling Topic Throughput Limits

If your queue-ti server has configured a `throughput_limit` on the topic you are consuming from, you may receive fewer messages than requested (or none at all) when the rate limit is exhausted. This is not an error — it is a **soft limit** that allows you to gracefully handle backpressure.

- **Single-message `Consume`**: When no messages are available or the throughput limit is exhausted, `Consume` receives `ErrNoMessage`, triggers the exponential backoff (500 ms → 30 s), and retries automatically. No changes to your code are needed.
- **Batch `ConsumeBatch`**: Similarly, when the limit is exhausted, `ConsumeBatch` receives 0 messages, applies exponential backoff, and polls again. Your handler is not called if there are no messages to process.

In both cases, the consumer automatically backs off and retries, making throughput-limited topics transparent to application code.

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

### `consumer.ConsumeBatch(ctx, topic string, batchSize int, handler) error`

Polls the queue in batches and dispatches each batch to the handler. Returns when `ctx` is cancelled.

```go
err := consumer.ConsumeBatch(ctx, "orders", 50, func(ctx context.Context, messages []*queueti.Message) error {
    // Process all messages in the batch.
    for _, msg := range messages {
        if err := process(msg); err != nil {
            // Nack this individual message.
            if err := msg.Nack(ctx, err.Error()); err != nil {
                log.Printf("nack failed: %v", err)
            }
            continue
        }
        // Ack this individual message.
        if err := msg.Ack(ctx); err != nil {
            log.Printf("ack failed: %v", err)
        }
    }
    return nil // handler always returns nil; ack/nack per message instead
})
```

**Batch semantics:**

- `batchSize`: number of messages to request per poll (valid range 1–1000; behavior is undefined outside this range).
- **Best-effort:** returns 0–N messages per call, never blocks or waits for a full batch. When the queue is empty or throughput-limited, the consumer applies the same exponential backoff as `Consume` (500 ms → 30 s). When messages are returned, backoff resets.
- **Per-message ack/nack:** each message in the slice has individual `Ack()` and `Nack(reason)` closures. Call them directly to acknowledge or reject each message, rather than returning an error from the handler.
- **Reconnection & backoff:** network errors are retried with exponential backoff (500 ms → 30 s), same as `Consume`.
- **Throughput limiting:** if the topic has a throughput limit configured, `ConsumeBatch` receives fewer messages than requested (or none) when the limit is exhausted; the consumer automatically backs off and retries.

Use `ConsumeBatch` when you want to process multiple messages together (e.g. batch writes to a data warehouse) or when you need more control over per-message error handling.

---

## Consumer Groups

Consumer groups enable independent consumption of the same messages by multiple systems. Each group tracks its own delivery state, allowing parallel processing of the same message by different applications without interference.

When a consumer group is specified, the client sends all RPCs scoped to that group and receives all messages enqueued to the topic. Each message is delivered independently to each group. A message is only deleted from the queue when **all** registered groups have acknowledged it.

### Registering a Consumer Group

Consumer groups must be registered on the server before use:

```bash
curl -X POST http://localhost:8080/api/topics/orders/consumer-groups \
  -H "Content-Type: application/json" \
  -d '{"consumer_group": "warehouse"}'
```

Once registered, the group automatically receives all pending messages enqueued before registration (backfill), plus all future messages.

### Single-Consumer (Consume)

```go
consumer := client.NewConsumer("orders",
    queueti.WithConsumerGroup("warehouse"),
    queueti.WithConcurrency(4),
)

err := consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    fmt.Printf("[warehouse] processing %s\n", msg.ID)
    return nil // nil = Ack; error = Nack
})
```

### Batch Consumer (ConsumeBatch)

`WithConsumerGroup` is set on `NewConsumer`; the consumer carries the group for all subsequent calls:

```go
consumer := client.NewConsumer("orders", queueti.WithConsumerGroup("warehouse"))

err := consumer.ConsumeBatch(ctx, "orders", 50,
    func(ctx context.Context, messages []*queueti.Message) error {
        for _, msg := range messages {
            if err := process(msg); err != nil {
                msg.Nack(ctx, err.Error())
                continue
            }
            msg.Ack(ctx)
        }
        return nil
    },
)
```

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
    Key        *string  // deduplication key (if message was published with WithKey)
}
```

- `ID`: The assigned message ID.
- `Topic`: The topic the message was enqueued on.
- `Payload`: The message body.
- `Metadata`: Arbitrary key-value metadata attached at publish time.
- `CreatedAt`: The timestamp when the message was first enqueued.
- `RetryCount`: The number of times this message has been nacked (0 on first receive).
- `Key`: The deduplication key (if one was provided at publish time); `nil` if the message was published without a key.

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

    queueti "github.com/Joessst-Dev/queue-ti/clients/go"
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
