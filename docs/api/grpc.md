# gRPC Service Reference

The gRPC service implements the `QueueService` defined in `proto/queue.proto`. All methods require JWT auth if enabled.

## Service Definition

The gRPC service provides four primary operations for queue clients:

- `Enqueue` — Add a message to a topic
- `Dequeue` — Retrieve the next message from a topic
- `Ack` — Acknowledge a processed message
- `Nack` — Signal that a message failed to process
- `BatchDequeue` — Retrieve multiple messages in one RPC call

## Enqueue

Enqueues a message to a topic.

```protobuf
rpc Enqueue(EnqueueRequest) returns (EnqueueResponse);

message EnqueueRequest {
  string topic = 1;                    // Topic name (required)
  bytes payload = 2;                   // Message payload (required)
  map<string, string> metadata = 3;    // Optional metadata
  optional string key = 4;             // Optional deduplication key
}

message EnqueueResponse {
  string id = 1;  // UUID of the enqueued message
}
```

**Key behavior** — See [Message Keys and Upsert Semantics](../guide/concepts#message-keys-and-upsert-semantics) for details on deduplication and upsert logic.

## Dequeue

Dequeues the next available message from a topic.

```protobuf
rpc Dequeue(DequeueRequest) returns (DequeueResponse);

message DequeueRequest {
  string topic = 1;                           // Topic name (required)
  optional uint32 visibility_timeout_seconds = 2;  // Visibility timeout override (optional, > 0 if set)
}

message DequeueResponse {
  string id = 1;                        // Message UUID
  string topic = 2;                     // Topic name
  bytes payload = 3;                    // Message payload
  map<string, string> metadata = 4;     // Metadata
  google.protobuf.Timestamp created_at = 5;  // Creation timestamp
  int32 retry_count = 6;                // Current retry count
  int32 max_retries = 7;                // Maximum retries for this message
  optional string key = 8;              // Deduplication key (if present)
}
```

Returns `codes.NotFound` if no messages are available; otherwise returns the next message and transitions it to `'processing'` status with a visibility timeout.

**Visibility Timeout Behavior**:
- When `visibility_timeout_seconds` is **omitted or not set**, the server-wide `visibility_timeout` configuration is used (default 30 seconds).
- When `visibility_timeout_seconds` is **set to a value > 0**, that duration (in seconds) overrides the server-wide configuration for this dequeue operation only.
- When `visibility_timeout_seconds` is **set to 0**, the request is rejected with `codes.InvalidArgument`.

## BatchDequeue

Dequeues up to N messages from a topic in a single round-trip. Returns immediately with however many messages are available (0 to N); never blocks waiting for messages.

```protobuf
rpc BatchDequeue(BatchDequeueRequest) returns (BatchDequeueResponse);

message BatchDequeueRequest {
  string topic = 1;                           // Topic name (required)
  uint32 count = 2;                           // Number of messages to dequeue (required, 1–1000)
  optional uint32 visibility_timeout_seconds = 3;  // Visibility timeout override (optional, > 0 if set)
}

message BatchDequeueResponse {
  repeated DequeueResponse messages = 1;      // Dequeued messages (0 to N)
}
```

**Error conditions**:
- `codes.InvalidArgument` if `count` is 0 or exceeds 1000

**Visibility Timeout Behavior** (same as single `Dequeue`):
- When `visibility_timeout_seconds` is **omitted or not set**, the server-wide `visibility_timeout` configuration is used.
- When `visibility_timeout_seconds` is **set to a value > 0**, that duration overrides the server-wide configuration for this batch dequeue only.
- When `visibility_timeout_seconds` is **set to 0**, the request is rejected with `codes.InvalidArgument`.

**Performance notes**:
- All returned messages are locked with `FOR UPDATE SKIP LOCKED`, preventing concurrent consumers from acquiring the same messages.
- Returns immediately with available messages even if fewer than requested; never blocks.
- Efficient for high-throughput batch processing scenarios.

**Throughput Limiting**:
- If the topic has a `throughput_limit` configured, `BatchDequeue` respects that rate limit
- The response may contain fewer messages than requested (including 0) when the rate limit is exhausted
- This is a **soft limit** — the operation succeeds and returns available messages rather than blocking or erroring

**Key field**: Each message in the response includes its optional `key` field (if present), allowing batch handlers to correlate messages with deduplication keys.

## Ack

Acknowledges (deletes) a processing message.

```protobuf
rpc Ack(AckRequest) returns (AckResponse);

message AckRequest {
  string id = 1;  // Message UUID (required)
}

message AckResponse {}
```

Fails if the message is not found or not in `'processing'` status.

## Nack

Signals that processing of a message failed and should be retried (if retries remain), promoted to the dead-letter queue (if DLQ threshold is reached), or marked failed.

```protobuf
rpc Nack(NackRequest) returns (NackResponse);

message NackRequest {
  string id = 1;          // Message UUID (required)
  string error = 2;       // Error description (optional, stored in last_error)
}

message NackResponse {}
```

Behavior depends on the DLQ threshold and retry count:
- If `retry_count + 1 >= dlq_threshold` (and `dlq_threshold > 0`), the message is **promoted to the dead-letter queue** (`<topic>.dlq`). Its `original_topic` is recorded, `max_retries` is set to 0, `retry_count` resets to 0, and status becomes `'pending'` in the DLQ topic.
- Otherwise, if `retry_count + 1 < max_retries`, its status reverts to `'pending'` and `retry_count` is incremented.
- Otherwise, its status becomes `'failed'`.

Fails if the message is not found or not in `'processing'` status.

## Authentication

For gRPC clients, include the JWT token in the `authorization` metadata header:

```go
import "google.golang.org/grpc/metadata"

// After login via HTTP to get the token
md := metadata.Pairs("authorization", "Bearer "+token)
ctx := metadata.NewOutgoingContext(context.Background(), md)

// Use ctx in gRPC calls
response, err := client.Enqueue(ctx, &pb.EnqueueRequest{...})
```

See [Authentication](../guide/authentication) for details on obtaining JWT tokens.

## Error Codes

The gRPC service returns standard gRPC error codes:

| Code | Meaning |
|------|---------|
| `codes.OK` | Success |
| `codes.NotFound` | Message or topic not found |
| `codes.InvalidArgument` | Invalid request parameters (e.g., count > 1000, visibility_timeout_seconds = 0) |
| `codes.FailedPrecondition` | Topic not registered (when `require_topic_registration` is enabled) |
| `codes.Unauthenticated` | Missing or invalid JWT token (when auth is enabled) |
| `codes.PermissionDenied` | User lacks required grant for this topic |
