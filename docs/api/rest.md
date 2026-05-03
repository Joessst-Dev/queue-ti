# REST API Reference

All HTTP endpoints are authenticated via JWT if enabled. See [Authentication](../guide/authentication) for details on obtaining and using tokens.

## Health Check

### GET /healthz

Health check endpoint. Always returns 200 OK.

```bash
curl http://localhost:8080/healthz
```

## Message Operations

### GET /api/messages

Lists all messages, optionally filtered by topic.

**Query Parameters:**
- `topic` (optional) — Filter by topic name

**Response:** Array of messages in reverse chronological order (newest first).

```bash
# List all messages
curl http://localhost:8080/api/messages

# Filter by topic
curl http://localhost:8080/api/messages?topic=orders
```

**Response body:**

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "topic": "orders",
    "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
    "metadata": {"user_id": "42"},
    "key": "order-123",
    "status": "pending",
    "retry_count": 0,
    "max_retries": 3,
    "created_at": "2025-04-25T12:00:00Z"
  }
]
```

### POST /api/messages

Enqueues a message.

**Request body:**

```json
{
  "topic": "orders",
  "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
  "metadata": {"user_id": "42"},
  "key": "order-123"
}
```

**Fields:**
- `topic` (string, required) — Topic name
- `payload` (string, required) — Base64-encoded message payload
- `metadata` (object, optional) — Key-value metadata
- `key` (string, optional) — Deduplication key for upsert semantics

**Example:**

```bash
# Enqueue without key (always creates new message)
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "orders",
    "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
    "metadata": {"user_id": "42"}
  }'

# Enqueue with key (upserts pending messages)
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "orders",
    "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
    "key": "order-123"
  }'
```

**Response:** HTTP 201 Created

```json
{"id": "550e8400-e29b-41d4-a716-446655440000"}
```

### POST /api/messages/dequeue

Dequeues up to N messages from a topic in a single request.

**Request body:**

```json
{
  "topic": "orders",
  "count": 10,
  "visibility_timeout_seconds": 30
}
```

**Fields:**
- `topic` (string, required) — Topic name
- `count` (uint32, optional) — Number of messages to dequeue (1–1000); defaults to 1 if omitted
- `visibility_timeout_seconds` (uint32, optional) — Visibility timeout override; if omitted, server-wide default applies

**Example:**

```bash
# Dequeue up to 10 messages
curl -X POST http://localhost:8080/api/messages/dequeue \
  -H "Content-Type: application/json" \
  -d '{"topic": "orders", "count": 10}'

# Dequeue with custom visibility timeout
curl -X POST http://localhost:8080/api/messages/dequeue \
  -H "Content-Type: application/json" \
  -d '{"topic": "orders", "count": 5, "visibility_timeout_seconds": 60}'
```

**Response:** HTTP 200 OK

```json
{
  "messages": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "topic": "orders",
      "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
      "metadata": {"user_id": "42"},
      "key": "order-123",
      "created_at": "2025-04-25T12:00:00Z",
      "retry_count": 0,
      "max_retries": 3
    }
  ]
}
```

**Behavior:**
- Returns 0 to N messages depending on availability; never blocks
- All returned messages transition to `'processing'` status with visibility timeout
- The `key` field is included in each message response (null if not present)

**Errors:**
- HTTP 400 if `count` is 0 or exceeds 1000
- HTTP 401 if authentication is enabled but no valid token is provided
- HTTP 422 if the topic is unregistered (when `require_topic_registration` is enabled)

### POST /api/messages/:id/nack

Signals that processing of a message failed.

```bash
curl -X POST http://localhost:8080/api/messages/:id/nack \
  -H "Content-Type: application/json" \
  -d '{"error": "connection timeout"}'
```

The `error` field is optional. If provided, it is stored in the message's `last_error` field.

**Response:** HTTP 204 No Content on success.

**Behavior**: If the message has retries remaining and has not reached the DLQ threshold, its status reverts to `'pending'` and it can be dequeued again. If the DLQ threshold is reached, the message is promoted to the dead-letter queue. Otherwise, its status becomes `'failed'`.

### POST /api/messages/:id/requeue

Moves a dead-letter queue message back to its original topic for reprocessing.

```bash
curl -X POST http://localhost:8080/api/messages/:id/requeue
```

**Response:** HTTP 204 No Content on success.

**Behavior**: Restores the message to its original topic (retrieved from `original_topic`), resets `retry_count` to 0, restores `max_retries` to the configured default, and sets status to `'pending'`.

Returns HTTP 404 if the message is not found or is not a dead-letter message.

### DELETE /api/topics/:topic/messages/by-key/:key

Deletes all messages with the given key on a topic, regardless of status. This is an administrative operation for purging duplicate or stale keyed messages.

**Request:**

```bash
curl -X DELETE -u admin:secret http://localhost:8080/api/topics/orders/messages/by-key/order-123
```

**Path parameters:**
- `topic` (string, required) — Topic name
- `key` (string, required) — Deduplication key to delete

**Response:** HTTP 200 OK

```json
{"deleted": 1}
```

The response indicates how many messages were deleted.

**Behavior:**
- Deletes **all** messages with the given `(topic, key)` pair regardless of their status
- Returns HTTP 404 if no messages with that key are found on the topic
- Admin-only endpoint (requires `is_admin=true` or basic auth)

## Topic Configuration

### GET /api/topic-configs

Lists all topic-level configuration overrides.

```bash
curl -u admin:secret http://localhost:8080/api/topic-configs
```

**Response:**

```json
{
  "items": [
    {
      "topic": "orders",
      "max_retries": 5,
      "message_ttl_seconds": 3600,
      "max_depth": 1000,
      "throughput_limit": 100
    }
  ]
}
```

### PUT /api/topic-configs/:topic

Creates or updates a topic-level configuration. Omitting a field or sending `null` reverts that setting to the global default.

```bash
curl -u admin:secret -X PUT http://localhost:8080/api/topic-configs/orders \
  -H "Content-Type: application/json" \
  -d '{
    "max_retries": 5,
    "message_ttl_seconds": 3600,
    "max_depth": 1000,
    "throughput_limit": 100
  }'
```

**Response:** HTTP 200 OK

### DELETE /api/topic-configs/:topic

Deletes a topic-level configuration, reverting all settings to global defaults.

```bash
curl -X DELETE -u admin:secret http://localhost:8080/api/topic-configs/orders
```

**Response:** HTTP 204 No Content

## Statistics

### GET /api/stats

Returns the current message count per topic and status (live queue depth).

```bash
curl -u admin:secret http://localhost:8080/api/stats
```

**Response:**

```json
{
  "topics": [
    {"topic": "orders", "status": "pending", "count": 5},
    {"topic": "orders", "status": "processing", "count": 2}
  ]
}
```

## Admin Operations

### POST /api/topics/:topic/purge

Permanently deletes messages from a topic, optionally filtered by status.

**Request body:**

```json
{
  "statuses": ["pending", "processing", "expired"]
}
```

The `statuses` array specifies which message statuses to delete. Omitting `statuses` or sending an empty array defaults to all three: `["pending", "processing", "expired"]`.

**Example:**

```bash
# Purge all pending, processing, and expired messages in orders topic
curl -X POST -u admin:secret http://localhost:8080/api/topics/orders/purge \
  -H "Content-Type: application/json" \
  -d '{"statuses": ["pending", "processing", "expired"]}'

# Purge only expired messages
curl -X POST -u admin:secret http://localhost:8080/api/topics/orders/purge \
  -H "Content-Type: application/json" \
  -d '{"statuses": ["expired"]}'
```

**Response:** HTTP 200 OK

```json
{"deleted": 42}
```

### POST /api/admin/expiry-reaper/run

Manually triggers the expiry reaper, which marks all messages with a passed `expires_at` timestamp as `expired`.

**Request body:** None (empty POST body)

**Example:**

```bash
curl -X POST -u admin:secret http://localhost:8080/api/admin/expiry-reaper/run
```

**Response:** HTTP 200 OK

```json
{"expired": 12}
```

### POST /api/admin/delete-reaper/run

Manually triggers the delete reaper, which permanently deletes all messages with `status = 'expired'`.

**Request body:** None (empty POST body)

**Example:**

```bash
curl -X POST -u admin:secret http://localhost:8080/api/admin/delete-reaper/run
```

**Response:** HTTP 200 OK

```json
{"deleted": 12}
```

### GET /api/admin/delete-reaper/schedule

Returns the current delete reaper cron schedule and activation status.

**Request:**

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/admin/delete-reaper/schedule
```

**Response:** HTTP 200 OK

```json
{
  "schedule": "0 2 * * *",
  "active": true
}
```

### PUT /api/admin/delete-reaper/schedule

Updates the delete reaper cron schedule. The new schedule is persisted to the database, applied immediately to the running instance, and will be used on future server restarts.

**Request body:**

```json
{
  "schedule": "0 */6 * * *"
}
```

**Example:**

```bash
# Change to every 6 hours
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"schedule": "0 */6 * * *"}' \
  http://localhost:8080/api/admin/delete-reaper/schedule

# Disable the delete reaper
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"schedule": ""}' \
  http://localhost:8080/api/admin/delete-reaper/schedule
```

**Response:** HTTP 200 OK

```json
{
  "schedule": "0 */6 * * *",
  "active": true
}
```

## Metrics

### GET /metrics

Prometheus metrics endpoint (unauthenticated).

```bash
curl http://localhost:8080/metrics
```

Returns Prometheus-format metrics. See [Observability](../guide/observability) for details.
