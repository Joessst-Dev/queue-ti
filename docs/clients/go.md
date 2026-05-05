# Go Client

The `clients/go-client/` package provides a high-level Producer/Consumer API for building Go applications that enqueue and dequeue messages from queue-ti's gRPC service.

## Installation

```bash
go get github.com/Joessst-Dev/queue-ti/clients/go-client
```

Or pin to a specific version:

```bash
go get github.com/Joessst-Dev/queue-ti/clients/go-client@v2026.05.0
```

## Quick Start

### Single-Message Consumer

```go
// Connect — token refreshes automatically before expiry
c, _ := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken(initialToken),
    queueti.WithTokenRefresher(fetchFreshToken),
)
defer c.Close()

// Publish
producer := c.NewProducer()
id, _ := producer.Publish(ctx, "orders", []byte(`{"amount":99}`))

// Publish with a deduplication key (upserts pending messages with same key)
id, _ := producer.Publish(ctx, "orders", []byte(`{"amount":99}`), 
    queueti.WithKey("order-123"),
)

// Consume (blocks until ctx cancelled)
consumer := c.NewConsumer("orders", queueti.WithConcurrency(4))
consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    fmt.Println(string(msg.Payload))
    return nil // nil = Ack, error = Nack
})
```

### Batch Consumer

For high-throughput scenarios, use batch dequeue to consume multiple messages in a single RPC call:

```go
c, _ := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken(initialToken),
)
defer c.Close()

consumer := c.NewConsumer("orders", queueti.WithConcurrency(4))

// ConsumeBatch dequeues up to batchSize messages and calls handler once
// with all messages. Each message has individual Ack() and Nack() closures.
consumer.ConsumeBatch(ctx, 10, func(ctx context.Context, messages []*queueti.BatchMessage) error {
    for _, msg := range messages {
        if err := processOrder(msg.Payload); err != nil {
            msg.Nack("processing failed: " + err.Error())
            continue
        }
        msg.Ack()
    }
    return nil // Handler return value is for fatal errors; individual messages use Ack/Nack
})
```

## API Reference

### Dial

Establishes a connection to the queue-ti gRPC server.

```go
c, err := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),           // Plaintext (no TLS)
    queueti.WithBearerToken(token),   // JWT token
    queueti.WithTokenRefresher(fn),   // Token refresh function
)
defer c.Close()
```

**Options:**
- `WithInsecure()` — Use plaintext instead of TLS (for local development)
- `WithBearerToken(token)` — Set initial JWT token for auth
- `WithTokenRefresher(func(ctx context.Context) (string, error))` — Function to refresh JWT tokens before expiry

### Producer

#### Publish

Enqueues a message to a topic.

```go
id, err := producer.Publish(ctx, "orders", []byte(`{"amount":99}`),
    queueti.WithKey("order-123"),       // Optional deduplication key
    queueti.WithMetadata(map[string]string{"user": "42"}), // Optional metadata
)
```

**Return:** Message UUID as string

**Options:**
- `WithKey(key)` — Set a deduplication key for upsert semantics
- `WithMetadata(metadata)` — Attach optional metadata

### Consumer

#### Consume

Consumes messages one at a time from a topic.

```go
err := consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    // Process message
    fmt.Println(string(msg.Payload))
    return nil  // Ack; return error to Nack
})
```

**Message fields:**
- `ID` (string) — Message UUID
- `Topic` (string) — Topic name
- `Payload` ([]byte) — Message payload
- `Metadata` (map[string]string) — Message metadata
- `CreatedAt` (time.Time) — Enqueue timestamp
- `RetryCount` (int) — Current retry count
- `MaxRetries` (int) — Maximum retries allowed
- `Key` (string, optional) — Deduplication key (if present)

**Behavior:**
- Blocks until context is cancelled or handler returns a non-recoverable error
- Auto-reconnects on connection loss
- Auto-refreshes JWT tokens before expiry
- Returns handler error (or fatal errors) as the error value

**Consumer options:**
- `WithConcurrency(n)` — Number of parallel dequeue goroutines (default: 1)
- `WithConsumerGroup(group)` — Consumer group name for group-based consumption
- `WithVisibilityTimeout(duration)` — Override default visibility timeout per dequeue

#### ConsumeBatch

Consumes messages in batches for higher throughput.

```go
err := consumer.ConsumeBatch(ctx, 10, func(ctx context.Context, messages []*queueti.BatchMessage) error {
    for _, msg := range messages {
        if err := processOrder(msg.Payload); err != nil {
            msg.Nack("processing failed: " + err.Error())
            continue
        }
        msg.Ack()
    }
    return nil  // Return error only for fatal handler errors
})
```

**BatchMessage fields and methods:**
- `Payload` ([]byte) — Message content
- `Metadata` (map[string]string) — Message metadata
- `CreatedAt` (time.Time) — Enqueue timestamp
- `RetryCount` (int) — Current retry count
- `MaxRetries` (int) — Maximum retries allowed
- `Ack()` — Acknowledge the message (removes it from the queue)
- `Nack(reason string)` — Nack the message (optionally with error reason); triggers retry or DLQ promotion

**ConsumeBatch behavior**:
- Dequeues up to `batchSize` messages (1–1000) in a single gRPC call
- Returns immediately with available messages (0 to batchSize); never blocks waiting for more
- Each message in the batch is individually locked and can be acked or nacked independently
- Auto-reconnect and token refresh work the same as single-message `Consume`

**Consumer options:**
- `WithConcurrency(n)` — Number of parallel batch dequeue goroutines (default: 1)
- `WithConsumerGroup(group)` — Consumer group name for group-based consumption
- `WithVisibilityTimeout(duration)` — Override default visibility timeout per batch dequeue

## Error Handling

The client handles errors gracefully:

```go
consumer := c.NewConsumer("orders")

err := consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    if err := process(msg); err != nil {
        return fmt.Errorf("processing failed: %w", err)  // Nack with error
    }
    return nil  // Ack
})

if err != nil {
    log.Fatalf("Consumer error: %v", err)  // Fatal error or context cancellation
}
```

**Common errors:**
- `context.DeadlineExceeded` — Context timeout
- `context.Canceled` — Context cancelled
- `grpc.Unavailable` — Connection lost (auto-reconnect will retry)
- `grpc.Unauthenticated` — Invalid or expired JWT token (auto-refresh will retry)

## Authentication

### Using QueueTiAuth (recommended)

The `NewAuth` helper automatically checks if authentication is required and handles login and token refresh:

```go
auth, err := queueti.NewAuth("http://localhost:8080", "admin", "secret")
if err != nil {
    log.Fatal(err)
}

opts := []queueti.DialOption{queueti.WithInsecure()}
if auth.Token() != "" {
    opts = append(opts,
        queueti.WithBearerToken(auth.Token()),
        queueti.WithTokenRefresher(auth.Refresh),
    )
}

client, err := queueti.Dial("localhost:50051", opts...)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

adminClient := queueti.NewAdminClient("http://localhost:8080",
    queueti.WithAdminToken(auth.Token()),
)
```

The `NewAuth` helper:
1. Calls `GET /api/auth/status` to check if authentication is required
2. If auth is disabled, returns a no-op instance with an empty token
3. If auth is enabled, calls `POST /api/auth/login` with the provided credentials
4. Exposes `Token()` for the current JWT and `Refresh(ctx)` which satisfies the `TokenRefresher` interface for automatic token refresh

### With JWT Tokens (manual)

```go
// Login to get initial token
token, err := login("admin", "secret")

// Create client with token and refresh function
c, err := queueti.Dial("localhost:50051",
    queueti.WithInsecure(),
    queueti.WithBearerToken(token),
    queueti.WithTokenRefresher(func(ctx context.Context) (string, error) {
        return refreshToken(ctx, token)  // Your refresh logic
    }),
)
defer c.Close()
```

## Consumer Groups

Use consumer groups to allow multiple independent systems to process the same messages:

```go
// Two groups, both consuming the same topic
warehouse := c.NewConsumer("orders",
    queueti.WithConsumerGroup("warehouse"),
    queueti.WithConcurrency(4),
)

analytics := c.NewConsumer("orders",
    queueti.WithConsumerGroup("analytics"),
    queueti.WithConcurrency(2),
)

// Each group independently processes all messages
```

See [Consumer Groups](../guide/consumer-groups) for details.

## Admin API

The `AdminClient` provides programmatic management of topic configuration, schemas, and consumer groups through the HTTP admin API on port 8080.

### Setup

```go
admin := queueti.NewAdminClient("http://localhost:8080",
    queueti.WithAdminToken("your-jwt-token"),
)
```

**Options:**
- `WithAdminToken(token)` — Set JWT token for authenticated requests
- `WithAdminHTTPClient(client)` — Replace the default HTTP client

### Example: Topic Configuration

```go
// List all topic configs
configs, err := admin.ListTopicConfigs(ctx)
if err != nil {
    log.Fatal(err)
}

// Set per-topic overrides
maxRetries, ttl := 5, 3600
_, err = admin.UpsertTopicConfig(ctx, "orders", queueti.TopicConfig{
    Topic:             "orders",
    MaxRetries:        &maxRetries,
    MessageTTLSeconds: &ttl,
    Replayable:        true,
})

// Delete a topic config (reverts to defaults)
err = admin.DeleteTopicConfig(ctx, "orders")
```

### Error Handling

```go
import "errors"

_, err := admin.ListTopicConfigs(ctx)
if errors.Is(err, queueti.ErrNotFound) {
    // Handle HTTP 404
} else if errors.Is(err, queueti.ErrConflict) {
    // Handle HTTP 409
}
```

### Full API

The `AdminClient` covers:
- **Topic configs**: `ListTopicConfigs`, `UpsertTopicConfig`, `DeleteTopicConfig`
- **Topic schemas**: `ListTopicSchemas`, `GetTopicSchema`, `UpsertTopicSchema`, `DeleteTopicSchema`
- **Consumer groups**: `ListConsumerGroups`, `RegisterConsumerGroup`, `UnregisterConsumerGroup`
- **Statistics**: `Stats()`

For complete examples and method signatures, see [clients/go-client/admin.go](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/go-client/admin.go).

## Sample Applications

### Order Pipeline

A self-contained end-to-end example demonstrating the full producer → consumer → ack lifecycle:

- Authentication via `NewAuth` — checks server auth status, logs in, and wires `TokenRefresher` automatically
- Consumer group registration via `AdminClient`
- Publishing messages with metadata and a deduplication key
- Streaming consumption with `concurrency=3`, ack on success, nack on failure (poison pill)
- DLQ drain — batch-polls `orders.dlq` and acks dead-lettered messages
- Graceful shutdown on SIGINT/SIGTERM via `signal.NotifyContext`

**Location**: [`clients/go-client/examples/order-pipeline/`](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/go-client/examples/order-pipeline)

```bash
# Requires: docker-compose up (from repo root)
# Credentials default to admin/secret; override with env vars:
# QUEUETI_USERNAME=admin QUEUETI_PASSWORD=secret go run .
go run .
```

## Full Client Documentation

For complete API reference and examples, see [clients/go-client/README.md](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/go-client).
