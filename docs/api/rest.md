# REST API Reference

All HTTP endpoints require a JWT bearer token when authentication is enabled. See [Authentication](../guide/authentication) for details on obtaining tokens. Endpoints marked **admin-only** additionally require `is_admin: true` on the authenticated user.

Error responses always have the shape `{"error": "message"}`.

## Version

### GET /api/version

Returns the running server version.

```bash
curl http://localhost:8080/api/version
```

**Response:** HTTP 200 OK

```json
{"version": "v2026.05.0"}
```

## Authentication

### GET /api/auth/status

Returns whether authentication is required on this server. No token needed.

```bash
curl http://localhost:8080/api/auth/status
```

**Response:** HTTP 200 OK

```json
{"auth_required": true}
```

### POST /api/auth/login

Exchange username and password for a JWT token. Rate-limited to 10 requests per IP per 60 seconds.

**Request body:**

```json
{"username": "admin", "password": "secret"}
```

**Response:** HTTP 200 OK

```json
{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}
```

**Errors:**
- HTTP 400 if `username` or `password` is missing
- HTTP 401 if credentials are invalid
- HTTP 429 if the rate limit is exceeded

### POST /api/auth/refresh

Exchanges a still-valid JWT for a freshly issued one with a new expiry. Requires a valid token.

```bash
curl -X POST -H "Authorization: Bearer <token>" http://localhost:8080/api/auth/refresh
```

**Response:** HTTP 200 OK

```json
{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}
```

## Message Operations

### GET /api/messages

Lists messages with optional topic filtering. Paginated.

**Query Parameters:**
- `topic` (optional) — Filter by topic name
- `limit` (optional) — Number of messages per page; default 50, max 200
- `offset` (optional) — Pagination offset; default 0

**Response:** HTTP 200 OK

```json
{
  "items": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "topic": "orders",
      "key": "order-123",
      "payload": "{\"order_id\": 12345}",
      "metadata": {"user_id": "42"},
      "status": "pending",
      "retry_count": 0,
      "max_retries": 3,
      "last_error": "",
      "expires_at": "2025-04-25T13:00:00Z",
      "created_at": "2025-04-25T12:00:00Z",
      "original_topic": "",
      "dlq_moved_at": null
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

**Message fields:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | UUID |
| `topic` | string | Topic name |
| `key` | string\|null | Deduplication key (null if not set) |
| `payload` | string | Message body as a string |
| `metadata` | object | Key-value metadata |
| `status` | string | `pending`, `processing`, `failed`, `expired` |
| `retry_count` | int | Number of previous delivery attempts |
| `max_retries` | int | Maximum retries configured for this message |
| `last_error` | string | Error string from the last nack (empty if none) |
| `expires_at` | string\|null | RFC 3339 expiry timestamp (null if no TTL) |
| `created_at` | string | RFC 3339 creation timestamp |
| `original_topic` | string | Set for DLQ messages; the topic they originated from |
| `dlq_moved_at` | string\|null | RFC 3339 timestamp when the message was moved to the DLQ |

**Example:**

```bash
curl -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/messages?topic=orders&limit=10&offset=0"
```

### POST /api/messages

Enqueues a message.

**Request body:**

```json
{
  "topic": "orders",
  "payload": "{\"order_id\": 12345}",
  "metadata": {"user_id": "42"},
  "key": "order-123"
}
```

**Fields:**
- `topic` (string, required) — Topic name
- `payload` (string, required) — Message body
- `metadata` (object, optional) — Key-value metadata
- `key` (string, optional) — Deduplication key; see [Message Keys](../guide/concepts#message-keys-and-upsert-semantics)

**Example:**

```bash
# Enqueue without key (always creates a new message)
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"topic": "orders", "payload": "{\"order_id\": 12345}", "metadata": {"user_id": "42"}}' \
  http://localhost:8080/api/messages

# Enqueue with key (upserts any existing pending message with the same key)
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"topic": "orders", "payload": "{\"order_id\": 12345}", "key": "order-123"}' \
  http://localhost:8080/api/messages
```

**Response:** HTTP 201 Created

```json
{"id": "550e8400-e29b-41d4-a716-446655440000"}
```

**Errors:**
- HTTP 422 if the payload fails schema validation or the topic is not registered
- HTTP 429 if the topic's `max_depth` limit is reached

### POST /api/messages/dequeue

Dequeues up to N messages from a topic.

**Request body:**

```json
{
  "topic": "orders",
  "count": 10,
  "visibility_timeout_seconds": 30,
  "consumer_group": "billing"
}
```

**Fields:**
- `topic` (string, required) — Topic name
- `count` (int, optional) — Number of messages to dequeue (1–1000); defaults to 1
- `visibility_timeout_seconds` (int, optional) — Override the server-wide visibility timeout for this call
- `consumer_group` (string, optional) — Consumer group name; when set, uses group-isolated dequeue

**Response:** HTTP 200 OK

```json
{
  "messages": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "topic": "orders",
      "key": "order-123",
      "payload": "{\"order_id\": 12345}",
      "metadata": {"user_id": "42"},
      "status": "processing",
      "retry_count": 0,
      "max_retries": 3,
      "created_at": "2025-04-25T12:00:00Z"
    }
  ]
}
```

**Behavior:**
- Returns 0 to N messages depending on availability; never blocks
- All returned messages transition to `processing` status with the visibility timeout applied
- Errors:
  - HTTP 400 if `count` is 0 or exceeds 1000
  - HTTP 422 if the topic is not registered (when `require_topic_registration` is enabled)

### POST /api/messages/:id/nack

Signals that processing of a message failed.

**Request body** (optional):

```json
{
  "error": "connection timeout",
  "consumer_group": "billing"
}
```

**Fields:**
- `error` (string, optional) — Error description, stored in the message's `last_error` field
- `consumer_group` (string, optional) — Must match the consumer group used during dequeue when group-isolated dequeue was used

**Response:** HTTP 204 No Content

**Behavior:**
- If `retry_count + 1 >= dlq_threshold` (and `dlq_threshold > 0`): message is promoted to the dead-letter queue (`<topic>.dlq`); the effective threshold is the per-topic `max_retries` when configured, otherwise the global `dlq_threshold`
- If retries remain: status reverts to `pending` and `retry_count` is incremented
- Otherwise: status becomes `failed`

### POST /api/messages/:id/requeue

Moves a dead-letter queue message back to its original topic for reprocessing.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/messages/550e8400-e29b-41d4-a716-446655440000/requeue
```

**Response:** HTTP 204 No Content

**Behavior:** Restores the message to its `original_topic`, resets `retry_count` to 0, restores `max_retries` to the effective DLQ threshold for that topic (per-topic `max_retries` if configured, otherwise the global `dlq_threshold`), and sets status to `pending`.

**Errors:**
- HTTP 404 if the message is not found or is not a dead-letter message

## Topic Configuration

### GET /api/topic-configs

Lists all topic-level configuration overrides. Admin-only.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/topic-configs
```

**Response:** HTTP 200 OK

```json
{
  "items": [
    {
      "topic": "orders",
      "max_retries": 5,
      "message_ttl_seconds": 3600,
      "max_depth": 1000,
      "throughput_limit": 100,
      "replayable": true,
      "replay_window_seconds": 86400
    }
  ]
}
```

**Fields** (all optional overrides — absent means the global default applies):

| Field | Type | Description |
|-------|------|-------------|
| `topic` | string | Topic name |
| `max_retries` | int\|null | Max delivery attempts before moving to DLQ; also overrides the global `dlq_threshold` for this topic |
| `message_ttl_seconds` | int\|null | Seconds until an unprocessed message expires |
| `max_depth` | int\|null | Maximum number of pending messages; enqueue returns HTTP 429 when full |
| `throughput_limit` | int\|null | Max messages dequeued per second (soft limit) |
| `replayable` | bool | Whether the topic retains an archive log for replay |
| `replay_window_seconds` | int\|null | How far back (in seconds) replay is allowed; only meaningful when `replayable` is true |

### PUT /api/topic-configs/:topic

Creates or updates a topic-level configuration. Admin-only. Omitting a field or sending `null` reverts that setting to the global default.

```bash
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "max_retries": 5,
    "message_ttl_seconds": 3600,
    "max_depth": 1000,
    "throughput_limit": 100,
    "replayable": true,
    "replay_window_seconds": 86400
  }' \
  http://localhost:8080/api/topic-configs/orders
```

**Response:** HTTP 200 OK — returns the stored config object (same shape as the list response items).

**Errors:**
- HTTP 400 if the topic name ends in `.dlq` (reserved) or `throughput_limit` is negative

### DELETE /api/topic-configs/:topic

Deletes a topic-level configuration, reverting all settings to global defaults. Admin-only.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/topic-configs/orders
```

**Response:** HTTP 204 No Content

## Topic Schema

### GET /api/topic-schemas

Lists all registered Avro schemas. Admin-only.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/topic-schemas
```

**Response:** HTTP 200 OK

```json
{
  "items": [
    {
      "topic": "orders",
      "schema_json": "{\"type\":\"record\",\"name\":\"Order\",...}",
      "version": 1,
      "updated_at": "2025-04-25T12:00:00Z"
    }
  ]
}
```

### GET /api/topic-schemas/:topic

Returns the schema for a single topic. Admin-only.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/topic-schemas/orders
```

**Response:** HTTP 200 OK — single schema object. HTTP 404 if no schema is registered for the topic.

### PUT /api/topic-schemas/:topic

Creates or replaces the Avro schema for a topic. Admin-only.

```bash
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"schema_json": "{\"type\":\"record\",\"name\":\"Order\",...}"}' \
  http://localhost:8080/api/topic-schemas/orders
```

**Response:** HTTP 200 OK — returns the stored schema object.

**Errors:**
- HTTP 400 if `schema_json` is not valid Avro JSON

### DELETE /api/topic-schemas/:topic

Removes the Avro schema for a topic. Subsequent enqueues are no longer validated. Admin-only.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/topic-schemas/orders
```

**Response:** HTTP 204 No Content

## Consumer Groups

### GET /api/topics/:topic/consumer-groups

Lists all registered consumer groups for a topic. Admin-only.

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/topics/orders/consumer-groups
```

**Response:** HTTP 200 OK

```json
{"items": ["billing", "fulfillment"]}
```

### POST /api/topics/:topic/consumer-groups

Registers a new consumer group on a topic. Admin-only.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"consumer_group": "billing"}' \
  http://localhost:8080/api/topics/orders/consumer-groups
```

**Response:** HTTP 201 Created (empty body)

**Errors:**
- HTTP 409 if the consumer group is already registered on this topic

### DELETE /api/topics/:topic/consumer-groups/:group

Unregisters a consumer group. Admin-only.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/topics/orders/consumer-groups/billing
```

**Response:** HTTP 204 No Content

**Errors:**
- HTTP 404 if the consumer group is not registered on this topic

## Replay and Message Log

### POST /api/topics/:topic/replay

Re-enqueues messages from the topic's archive log. The topic must have `replayable: true` in its config. Admin-only.

**Request body** (optional):

```json
{"from_time": "2025-04-25T00:00:00Z"}
```

`from_time` (RFC 3339, optional) — only replay messages created at or after this timestamp. Omit to replay from the beginning of the retention window.

**Response:** HTTP 200 OK

```json
{
  "topic": "orders",
  "enqueued": 42,
  "from_time": "2025-04-25T00:00:00Z"
}
```

`from_time` in the response is an empty string when the request did not include a `from_time`.

**Errors:**
- HTTP 422 if the topic is not replayable or `from_time` falls outside the replay window

### GET /api/topics/:topic/message-log

Returns the archived (acked) message log for a topic. Admin-only. Paginated.

**Query Parameters:**
- `limit` (optional) — default 50, max 200
- `offset` (optional) — default 0

**Response:** HTTP 200 OK

```json
{
  "items": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "topic": "orders",
      "key": "order-123",
      "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
      "retry_count": 0,
      "original_topic": "",
      "created_at": "2025-04-25T12:00:00Z",
      "acked_at": "2025-04-25T12:00:05Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

Note: `payload` in the message log is **base64-encoded**.

### DELETE /api/topics/:topic/message-log

Trims the message log by deleting entries older than the given timestamp. Admin-only.

**Query Parameters:**
- `before` (RFC 3339, required) — delete log entries with `acked_at` before this timestamp

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  "http://localhost:8080/api/topics/orders/message-log?before=2025-01-01T00:00:00Z"
```

**Response:** HTTP 200 OK

```json
{"deleted": 15}
```

## Statistics

### GET /api/stats

Returns the current message count per topic and status. Admin-only.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/stats
```

**Response:** HTTP 200 OK

```json
{
  "topics": [
    {"topic": "orders", "status": "pending", "count": 5},
    {"topic": "orders", "status": "processing", "count": 2}
  ]
}
```

## User Management

All user management endpoints are admin-only.

### GET /api/users

Lists all users.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/users
```

**Response:** HTTP 200 OK

```json
{
  "items": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "username": "billing-service",
      "is_admin": false,
      "created_at": "2025-04-25T12:00:00Z",
      "updated_at": "2025-04-25T12:00:00Z"
    }
  ]
}
```

### POST /api/users

Creates a new user.

**Request body:**

```json
{"username": "billing-service", "password": "minimum12chars!", "is_admin": false}
```

**Response:** HTTP 201 Created — returns the user object.

**Errors:**
- HTTP 400 if `username` or `password` is missing, or password is shorter than 12 characters
- HTTP 409 if the username is already taken

### PUT /api/users/:id

Updates a user. All fields are optional; only supplied fields are changed.

**Request body:**

```json
{"username": "new-name", "password": "newpassword123", "is_admin": true}
```

**Response:** HTTP 200 OK — returns the updated user object.

**Errors:**
- HTTP 400 if the new password is shorter than 12 characters
- HTTP 404 if the user does not exist
- HTTP 409 if the new username is already taken

### DELETE /api/users/:id

Deletes a user.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000
```

**Response:** HTTP 204 No Content

**Errors:**
- HTTP 400 if attempting to delete the currently authenticated user
- HTTP 404 if the user does not exist

### GET /api/users/:id/grants

Lists all grants for a user.

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/grants
```

**Response:** HTTP 200 OK

```json
{
  "items": [
    {
      "id": "grant-uuid",
      "user_id": "user-uuid",
      "action": "write",
      "topic_pattern": "orders*",
      "consumer_group": "",
      "created_at": "2025-04-25T12:00:00Z"
    }
  ]
}
```

### POST /api/users/:id/grants

Adds a topic grant to a user.

**Request body:**

```json
{"action": "write", "topic_pattern": "orders*"}
```

**Fields:**
- `action` (string, required) — `read`, `write`, or `admin`
- `topic_pattern` (string, optional) — glob pattern; defaults to `*` (all topics) if omitted

**Response:** HTTP 201 Created — returns the grant object.

### DELETE /api/users/:id/grants/:grantId

Removes a grant from a user.

**Response:** HTTP 204 No Content

**Errors:**
- HTTP 404 if the grant does not exist or does not belong to the user

### POST /api/users/:id/consumer-group-grants

Restricts a user's dequeue access to a specific consumer group on a topic pattern.

**Request body:**

```json
{"topic_pattern": "orders*", "consumer_group": "billing"}
```

**Fields:**
- `consumer_group` (string, required) — The consumer group the user is restricted to
- `topic_pattern` (string, optional) — Glob pattern; defaults to `*`

**Response:** HTTP 201 Created — returns the grant object.

**Errors:**
- HTTP 409 if the same consumer group grant already exists

## Admin Operations

All admin operations require `is_admin: true`.

### POST /api/topics/:topic/purge

Permanently deletes messages from a topic by status.

**Request body:**

```json
{"statuses": ["pending", "processing", "expired"]}
```

Omitting `statuses` or sending an empty array defaults to all three: `["pending", "processing", "expired"]`.

**Response:** HTTP 200 OK

```json
{"deleted": 42}
```

**Errors:**
- HTTP 400 if an unrecognised status is included (only `pending`, `processing`, `expired` are valid)

### DELETE /api/topics/:topic/messages/by-key/:key

Deletes all messages with the given key on a topic, regardless of status.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/topics/orders/messages/by-key/order-123
```

**Response:** HTTP 200 OK

```json
{"deleted": 1}
```

### POST /api/admin/expiry-reaper/run

Manually triggers the expiry reaper, which marks all messages whose `expires_at` has passed as `expired`.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/admin/expiry-reaper/run
```

**Response:** HTTP 200 OK

```json
{"expired": 12}
```

### POST /api/admin/delete-reaper/run

Manually triggers the delete reaper, which permanently deletes all messages with `status = expired`.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/admin/delete-reaper/run
```

**Response:** HTTP 200 OK

```json
{"deleted": 12}
```

### GET /api/admin/delete-reaper/schedule

Returns the current delete reaper cron schedule.

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/admin/delete-reaper/schedule
```

**Response:** HTTP 200 OK

```json
{"schedule": "0 2 * * *", "active": true}
```

`active` is `false` when `schedule` is an empty string (reaper disabled).

### PUT /api/admin/delete-reaper/schedule

Updates the delete reaper cron schedule. The new schedule is persisted and applied immediately.

**Request body:**

```json
{"schedule": "0 */6 * * *"}
```

Send an empty string to disable the reaper: `{"schedule": ""}`.

**Response:** HTTP 200 OK

```json
{"schedule": "0 */6 * * *", "active": true}
```

### POST /api/admin/archive-reaper/run

Manually triggers the archive reaper, which purges message log entries that have aged out of the configured replay window.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/admin/archive-reaper/run
```

**Response:** HTTP 200 OK

```json
{"deleted": 5}
```

## Metrics

### GET /metrics

Prometheus metrics endpoint. Requires JWT auth when authentication is enabled.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/metrics
```

Returns Prometheus-format metrics. See [Observability](../guide/observability) for details.

## Health Check

### GET /healthz

Health check endpoint. Always returns HTTP 200 OK. No authentication required.

```bash
curl http://localhost:8080/healthz
```
