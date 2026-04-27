# queue-ti

A distributed message queue built on PostgreSQL, gRPC, and HTTP. No Kafka, Redis, or RabbitMQ — if you have Postgres, you have a production-ready queue.

queue-ti is designed for teams who want reliable, observable message processing without the operational overhead of a separate queue broker. It features built-in dead-letter queues, per-topic schema validation, JWT authentication with fine-grained grants, and a browser-based admin UI out of the box. Consumers and producers connect via high-performance gRPC; operators manage the queue through a REST API and Angular dashboard.

## Why queue-ti?

- **PostgreSQL only** — No additional infrastructure. If you run Postgres already, queue-ti is a drop-in message queue with one table.
- **At-least-once delivery** — Messages are never lost. Visibility timeouts ensure unacked messages are retried. Dead-letter queue automatically contains exhausted messages for manual inspection and requeue.
- **Built for observability** — Prometheus metrics out of the box (`/metrics`); live queue depth via REST API; admin UI shows message status, retry counts, and expiry times.
- **High performance** — gRPC protocol with `FOR UPDATE SKIP LOCKED` dequeue. Throughput tested at 1500+ ops/sec per consumer.
- **Admin UI included** — Inspect messages, manually enqueue test data, requeue from DLQ, manage topics and users—all without writing code. OAuth-ready JWT auth.
- **Per-topic configuration** — Override retry limits, TTLs, and queue depth per topic at runtime without restart.
- **Avro schema validation** — Optional per-topic schemas enforce payload contracts at enqueue time.
- **Go client library** — Drop-in Producer/Consumer with auto-reconnect, token refresh, and zero boilerplate.

## Features

- **gRPC API** — High-performance queue operations (enqueue, dequeue, acknowledge, nack) over gRPC
- **HTTP Admin API** — REST endpoints for queue inspection, management, user/grant administration, and schema configuration
- **Topic-based routing** — Multiple independent queues share a single PostgreSQL table, partitioned by topic
- **Automatic retries** — Failed messages are automatically retried up to a configurable limit; consumers call `Nack` to signal failure
- **Dead-letter queue** — Messages that exhaust their retry limit are automatically promoted to `<topic>.dlq`; can be manually requeued to the original topic
- **Message TTL** — Messages expire after a configurable duration; an automatic reaper marks expired messages
- **Contention-free dequeue** — Uses `FOR UPDATE SKIP LOCKED` for lock-free concurrent consumption
- **JWT authentication** — Optional JWT-based auth (HS256) with user accounts, role-based access, and per-topic grants
- **Avro schema validation** — Optional per-topic Avro schema registration; payloads validated at enqueue time
- **Per-topic configuration** — Override retry count, TTL, and queue depth limits per topic via HTTP API or admin UI
- **Admin UI** — Angular web interface for message inspection, manual enqueue, DLQ requeue, and topic management
- **Prometheus metrics** — Real-time counters and gauges (`/metrics` endpoint, unauthenticated)
- **Go client library** — Drop-in `Producer` and `Consumer` with auto-reconnection and token refresh

## Table of Contents

- [Quick Start](#quick-start)
- [Go Client Library](#go-client-library)
- [Configuration](#configuration)
- [Authentication & User Management](#authentication--user-management)
- [Avro Schema Validation](#avro-schema-validation)
- [Queue Mechanics](#queue-mechanics)
- [Architecture](#architecture)
- [API Reference](#api-reference)
- [Observability](#observability)
- [Running Tests](#running-tests)
- [Performance Testing](#performance-testing)
- [Development Workflow](#development-workflow)
- [Project Structure](#project-structure)
- [Deployment](#deployment)
- [Release Management](#release-management)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

## Quick Start

### Prerequisites

- Go 1.25.5 or later
- PostgreSQL 16+
- Node.js 20+ (for admin UI development)
- Docker and Docker Compose (optional, for containerized deployment)

### Local Development

1. **Clone the repository**
   ```bash
   git clone https://github.com/Joessst-Dev/queue-ti
   cd queue-ti
   ```

2. **Set up PostgreSQL**
   ```bash
   # Using Docker
   docker run --rm -d \
     --name queueti-postgres \
     -e POSTGRES_DB=queueti \
     -e POSTGRES_USER=postgres \
     -e POSTGRES_PASSWORD=postgres \
     -p 5432:5432 \
     postgres:16-alpine

   # Wait for health check
   docker exec queueti-postgres pg_isready -U postgres
   ```

3. **Start the backend server**
   ```bash
   make run
   ```
   The server listens on:
   - **gRPC**: `localhost:50051` (for queue producers/consumers)
   - **HTTP**: `localhost:8080` (for admin UI and REST API)

4. **Start the admin UI** (in another terminal)
   ```bash
   cd admin-ui
   npm install
   npx nx serve
   ```
   The UI is available at `http://localhost:4200`

5. **Clean up**
   ```bash
   docker stop queueti-postgres
   ```

### Docker Compose

Deploy the full stack (PostgreSQL + backend + admin UI) with one command:

```bash
docker-compose up
```

The admin UI is available at `http://localhost:8081` (login: `admin` / `secret`).

## Go Client Library

The `client/` package provides a high-level Producer/Consumer API for building applications that enqueue and dequeue messages from queue-ti's gRPC service.

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

// Consume (blocks until ctx cancelled)
consumer := c.NewConsumer("orders", queueti.WithConcurrency(4))
consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    fmt.Println(string(msg.Payload))
    return nil // nil = Ack, error = Nack
})
```

See [client/README.md](client/README.md) for the full API reference, authentication setup, error handling, and examples.

## Configuration

Configuration is loaded from `config.yaml` at the repository root. All keys can be overridden with environment variables prefixed `QUEUETI_`.

### Configuration File

Create or edit `config.yaml`:

```yaml
server:
  port: 50051          # gRPC server port
  http_port: 8080      # HTTP admin API port

db:
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  name: queueti
  sslmode: disable     # Options: disable, require, verify-ca, verify-full

queue:
  visibility_timeout: 30s       # Time a dequeued message remains invisible to other consumers
  max_retries: 3                # Maximum number of retries for a failed message
  message_ttl: 24h              # Time-to-live for messages (0 = no expiry)
  dlq_threshold: 3              # Retry count at which messages are promoted to DLQ (0 = disabled)
  require_topic_registration: false  # Require explicit topic registration before enqueue (default: false)

auth:
  enabled: false
  username: admin
  password: secret

log_level: info         # Log level: debug, info, warn, error (default: info)
```

### Environment Variables

Any configuration key can be overridden with an environment variable using the key path with underscores and the `QUEUETI_` prefix:

| Variable | Description | Example |
|----------|-------------|---------|
| `QUEUETI_SERVER_PORT` | gRPC port | `50051` |
| `QUEUETI_SERVER_HTTP_PORT` | HTTP port | `8080` |
| `QUEUETI_DB_HOST` | PostgreSQL host | `localhost` |
| `QUEUETI_DB_PORT` | PostgreSQL port | `5432` |
| `QUEUETI_DB_USER` | PostgreSQL user | `postgres` |
| `QUEUETI_DB_PASSWORD` | PostgreSQL password | `postgres` |
| `QUEUETI_DB_NAME` | PostgreSQL database | `queueti` |
| `QUEUETI_DB_SSLMODE` | PostgreSQL SSL mode | `disable` |
| `QUEUETI_QUEUE_VISIBILITY_TIMEOUT` | Visibility timeout | `30s` |
| `QUEUETI_QUEUE_MAX_RETRIES` | Max retry count per message | `3` |
| `QUEUETI_QUEUE_MESSAGE_TTL` | Message time-to-live (0 = no expiry) | `24h` |
| `QUEUETI_QUEUE_DLQ_THRESHOLD` | Retry count for DLQ promotion (0 = disabled) | `3` |
| `QUEUETI_QUEUE_REQUIRE_TOPIC_REGISTRATION` | Require topics to be registered before enqueue | `false` |
| `QUEUETI_AUTH_ENABLED` | Enable JWT authentication | `true` |
| `QUEUETI_AUTH_JWT_SECRET` | JWT signing secret (required if auth enabled) | (any string) |
| `QUEUETI_AUTH_USERNAME` | Default admin username | `admin` |
| `QUEUETI_AUTH_PASSWORD` | Default admin password | `secret` |
| `QUEUETI_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

### Log Levels

The `log_level` configuration controls the verbosity of server logging:

| Level | Use Case | Typical Output |
|-------|----------|-----------------|
| **debug** | Local development, detailed message tracing | Per-message operations (enqueue, dequeue, ack, nack-retry), HTTP requests |
| **info** | Production (default) | Server startup, DLQ promotions, requeue operations, expiry reaper results, auth enabled notice |
| **warn** | Production monitoring | Authentication failures, DLQ threshold misconfiguration |
| **error** | Production incidents | Unexpected DB failures, server errors |

Set via environment variable:
```bash
QUEUETI_LOG_LEVEL=debug
```

Or in `config.yaml`:
```yaml
log_level: debug
```

The resolved log level is printed at server startup.

### Per-Topic Configuration

Individual topics can override the global queue settings. This is useful when certain topics require stricter retry limits, longer TTLs, or queue depth constraints.

**Supported overrides:**
- `max_retries` — Maximum retry count for messages on this topic (overrides global `max_retries`)
- `message_ttl_seconds` — Time-to-live for messages in seconds (overrides global `message_ttl`); set to `0` to disable TTL for this topic
- `max_depth` — Maximum number of pending+processing messages allowed on this topic; set to `null` or `0` for unlimited; `Enqueue` returns HTTP 429 when the topic reaches capacity

**Precedence:** Per-topic overrides take priority over global defaults. Omitting a field (or sending `null`) reverts that setting to the global default.

**Set per-topic configuration:**

```bash
curl -u admin:secret -X PUT http://localhost:8080/api/topic-configs/orders \
  -H "Content-Type: application/json" \
  -d '{
    "max_retries": 5,
    "message_ttl_seconds": 3600,
    "max_depth": 1000
  }'
```

Response:
```json
{
  "topic": "orders",
  "max_retries": 5,
  "message_ttl_seconds": 3600,
  "max_depth": 1000
}
```

**Clear an override to return to global default:**

```bash
curl -u admin:secret -X PUT http://localhost:8080/api/topic-configs/orders \
  -H "Content-Type: application/json" \
  -d '{"max_retries": null}'
```

Topics ending in `.dlq` (dead-letter queue topics) cannot have configurations set; the API returns HTTP 400 if you attempt to configure a DLQ topic.

The admin UI **Config** tab allows interactive viewing and editing of all topic configurations without server restart.

### Topic Registration

By default, queue-ti allows messages to be enqueued to any topic without prior registration. This is convenient for development but can be risky in production—typos in topic names create silent, unrecoverable message loss.

To require explicit topic registration, enable the `require_topic_registration` flag:

```yaml
queue:
  require_topic_registration: true
```

Or via environment variable:
```bash
QUEUETI_QUEUE_REQUIRE_TOPIC_REGISTRATION=true
```

**Behavior when registration is required:**
- Enqueue requests to unregistered topics are rejected with HTTP 422 (gRPC `FailedPrecondition`)
- Topics are registered by creating a configuration entry via `PUT /api/topic-configs/:topic`
- The admin UI **New Topic** button (in the Topics section) simplifies registration; when enabled, admins must register a topic before producers can enqueue to it
- The empty-state message in the admin UI changes to: "No topics registered. Use 'New Topic' to register a topic before messages can be enqueued to it."

**Example: Register a topic and enqueue a message**

```bash
# Register the topic
curl -u admin:secret -X PUT http://localhost:8080/api/topic-configs/orders \
  -H "Content-Type: application/json" \
  -d '{}'

# Now enqueue is allowed
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{"topic": "orders", "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ=="}'
```

**When to enable registration:**
- Production deployments where topic names are fixed and controlled
- Microservices architectures with schema registries (topics are registered alongside schemas)
- Teams that want producer errors on typos rather than silent failures

## Authentication & User Management

queue-ti supports JWT-based authentication with per-user grants to enforce granular access control. User accounts and role assignments are managed via the admin UI or REST API.

### Enabling Authentication

Authentication is disabled by default. To enable it, set:

```yaml
auth:
  enabled: true
  jwt_secret: "your-secret-key-here"
```

Or use environment variables:
```bash
QUEUETI_AUTH_ENABLED=true
QUEUETI_AUTH_JWT_SECRET="your-secret-key-here"
```

The server will fail to start if `auth.enabled=true` and `jwt_secret` is empty.

### Default Admin User

On first startup, the server automatically seeds a default admin user from the configuration:

```yaml
auth:
  enabled: true
  jwt_secret: "your-secret-key-here"
  username: admin          # Becomes the first admin account username
  password: secret         # Becomes the first admin account password
```

After the server starts, the default user is created (if it doesn't already exist) with `is_admin=true`. You can change the password and create additional users via the admin UI **Users** tab.

### User Roles and Permissions

#### Admin Flag

The `is_admin` flag grants a user unrestricted access:
- **Admin users** (`is_admin=true`) bypass all per-topic grant checks and can access all queue operations and admin endpoints
- **Regular users** (`is_admin=false`) are subject to per-topic grants

#### Per-Topic Grants

Regular users require explicit grants for each action and topic. A grant specifies:
- **Action**: one of `read`, `write`, or `admin`
- **Topic Pattern**: one of the following:
  - `*` — All topics (wildcard grant)
  - `orders.*` — Prefix glob (e.g., matches `orders`, `orders.dlq`, `orders.v1`, etc.)
  - `orders` — Exact topic name

**Grant semantics:**
- `read` — Allows `GET /api/messages` and `GET /api/stats` for this topic
- `write` — Allows `POST /api/messages`, `POST /api/messages/:id/ack`, `POST /api/messages/:id/nack`, `POST /api/messages/:id/requeue` for this topic; also required for gRPC `Dequeue` calls
- `admin` — Allows topic configuration and schema management (`GET/PUT/DELETE /api/topic-configs/:topic`, `GET/PUT/DELETE /api/topic-schemas/:topic`) for this topic

**Example grants:**

| Username | Grant | Topic Pattern | Interpretation |
|----------|-------|---------------|-----------------|
| `alice` | `write` | `orders` | Can enqueue, dequeue, ack, and nack messages only in the `orders` topic |
| `bob` | `read` | `*` | Can list and inspect all messages across all topics, but cannot modify any |
| `charlie` | `admin` | `payments.*` | Can manage configuration and schema for all topics matching `payments.*` (e.g., `payments`, `payments.dlq`, `payments.v1`) |

### JWT Token Details

- **Token lifetime**: 15 minutes
- **Algorithm**: HS256 (HMAC with SHA-256)
- **Claims**:
  - `uid` — User UUID
  - `sub` — Username
  - `adm` — Boolean indicating if user is admin
  - `iat` — Issued at (standard)
  - `exp` — Expiration time (standard)

### Authentication Endpoints

#### POST /api/auth/login

Authenticates a user and returns a JWT token.

**Request:**
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "secret"}'
```

**Response:** HTTP 200 OK
```json
{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}
```

#### POST /api/auth/refresh

Refreshes an existing JWT token.

**Request:**
```bash
curl -X POST http://localhost:8080/api/auth/refresh \
  -H "Authorization: Bearer <token>"
```

**Response:** HTTP 200 OK with a new token
```json
{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}
```

#### GET /api/auth/status

Returns the current authentication status and the authenticated user.

**Request:**
```bash
curl http://localhost:8080/api/auth/status
```

**Response:** HTTP 200 OK
```json
{
  "auth_required": true,
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "admin",
    "is_admin": true
  }
}
```

### User Management Endpoints (Admin-Only)

All user and grant management endpoints require `is_admin=true`.

#### GET /api/users

Lists all user accounts.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/users
```

**Response:**
```json
{
  "users": [
    {"id": "550e8400...", "username": "admin", "is_admin": true, "created_at": "2025-04-25T12:00:00Z"},
    {"id": "660e8400...", "username": "alice", "is_admin": false, "created_at": "2025-04-25T12:05:00Z"}
  ]
}
```

#### POST /api/users

Creates a new user account.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"username": "bob", "password": "secure-password", "is_admin": false}' \
  http://localhost:8080/api/users
```

**Response:** HTTP 201 Created
```json
{"id": "770e8400...", "username": "bob", "is_admin": false, "created_at": "2025-04-25T12:10:00Z"}
```

#### PUT /api/users/:id

Updates a user account (username, password, and/or admin flag).

```bash
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"is_admin": true}' \
  http://localhost:8080/api/users/770e8400-e29b-41d4-a716-446655440002
```

**Response:** HTTP 200 OK with the updated user

#### DELETE /api/users/:id

Deletes a user account.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/770e8400-e29b-41d4-a716-446655440002
```

**Response:** HTTP 204 No Content

### Grant Management Endpoints (Admin-Only)

#### GET /api/users/:id/grants

Lists all grants for a specific user.

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/grants
```

**Response:**
```json
{
  "grants": [
    {
      "id": "880e8400...",
      "user_id": "550e8400...",
      "action": "write",
      "topic_pattern": "orders",
      "created_at": "2025-04-25T12:00:00Z"
    }
  ]
}
```

#### POST /api/users/:id/grants

Creates a new grant for a user.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"action": "write", "topic_pattern": "payments.*"}' \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/grants
```

**Response:** HTTP 201 Created

#### DELETE /api/users/:id/grants/:grant_id

Deletes a specific grant.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/grants/880e8400-e29b-41d4-a716-446655440003
```

**Response:** HTTP 204 No Content

### Using JWT Tokens in HTTP Requests

After logging in, include the JWT token in the `Authorization: Bearer` header:

```bash
# Login to get token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "secret"}' | jq -r '.token')

# Use token for authenticated requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/messages
```

### Using JWT Tokens in gRPC Requests

For gRPC clients, include the JWT token in the `authorization` metadata header:

```go
import "google.golang.org/grpc/metadata"

// After login via HTTP to get the token
md := metadata.Pairs("authorization", "Bearer "+token)
ctx := metadata.NewOutgoingContext(context.Background(), md)

// Use ctx in gRPC calls
response, err := client.Enqueue(ctx, &pb.EnqueueRequest{...})
```

### Admin UI Authentication

The admin UI stores JWT tokens in `sessionStorage` after successful login. Tokens are automatically included in all HTTP requests via the auth interceptor.

**Login flow:**
1. User navigates to the login page
2. Enters username and password
3. Calls `POST /api/auth/login` with credentials
4. Token is stored in `sessionStorage`
5. User is redirected to the messages dashboard
6. Auth interceptor automatically adds `Authorization: Bearer <token>` to all subsequent requests

**Token expiration:**
- When a token expires (15 minutes), the next API request returns HTTP 401
- The admin UI prompts the user to log in again
- Users can manually refresh tokens via `POST /api/auth/refresh`

> **Security Note**: Never commit the JWT secret to version control. Use a `.env` file or a secrets management system (e.g., Docker secrets, Kubernetes secrets) in production.

## Avro Schema Validation

Topics can have an optional Avro schema registered. When a schema is registered for a topic, all `Enqueue` calls validate the JSON payload against that schema before storing the message. Topics without a registered schema accept any payload.

### How It Works

- **Schema registration**: Register an Avro schema for a topic via `PUT /api/topic-schemas/:topic`. The schema must be valid Avro JSON; invalid schemas are rejected with HTTP 400.
- **Validation at enqueue**: When a message is enqueued to a topic with a schema, the payload is validated as JSON and checked against the schema structure. If the payload does not conform, the enqueue fails with HTTP 422.
- **No schema = no validation**: Topics without a registered schema accept any payload. Existing messages are unaffected when a schema is added, updated, or removed.
- **Performance**: Parsed Avro schemas are cached in memory per topic. The cache automatically invalidates when a schema is updated or deleted.

### Validation Rules

For record schemas (the most common Avro type):
- Every required field (fields with no default value) must be present in the JSON payload
- Every present field must have a value compatible with its Avro type
- Optional fields (fields with a default value) may be omitted from the payload
- For other Avro types (primitives, arrays, maps, unions), the payload must be valid JSON and the top-level type must be compatible

### Schema Registration Endpoints

#### GET /api/topic-schemas

Lists all registered schemas.

```bash
curl -u admin:secret http://localhost:8080/api/topic-schemas
```

**Response:**
```json
{
  "items": [
    {
      "topic": "orders",
      "schema_json": "{\"type\":\"record\",\"name\":\"Order\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"total\",\"type\":\"float\"}]}",
      "version": 1,
      "updated_at": "2025-04-25T12:00:00Z"
    }
  ]
}
```

#### PUT /api/topic-schemas/:topic

Registers or updates an Avro schema for a topic. If a schema already exists, the version is incremented.

```bash
curl -u admin:secret -X PUT http://localhost:8080/api/topic-schemas/orders \
  -H "Content-Type: application/json" \
  -d '{
    "schema_json": "{\"type\":\"record\",\"name\":\"Order\",\"fields\":[{\"name\":\"order_id\",\"type\":\"int\"},{\"name\":\"customer_email\",\"type\":\"string\"},{\"name\":\"amount\",\"type\":\"double\"}]}"
  }'
```

**Response:** HTTP 200 OK

#### GET /api/topic-schemas/:topic

Fetches the schema registered for a specific topic.

```bash
curl -u admin:secret http://localhost:8080/api/topic-schemas/orders
```

**Response:** HTTP 200 OK or HTTP 404 if no schema is registered

#### DELETE /api/topic-schemas/:topic

Removes the registered schema for a topic. Existing messages are unaffected.

```bash
curl -u admin:secret -X DELETE http://localhost:8080/api/topic-schemas/orders
```

**Response:** HTTP 204 No Content

### Validation Errors

When a payload fails validation, the error includes details about what went wrong:

```json
{"error": "payload does not match topic schema: missing required field \"order_id\""}
```

Common validation error messages:
- `missing required field "fieldname"` — A required field is absent from the payload
- `field "fieldname": expected string, got number` — A field has the wrong JSON type
- `payload is not a valid JSON object` — The payload is not valid JSON or is not an object for a record schema

## Queue Mechanics

### Data Model

Messages are stored in a single `messages` PostgreSQL table with the following columns:

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `topic` | TEXT | Topic name (required) |
| `payload` | BYTEA | Message payload (required) |
| `metadata` | JSONB | Optional metadata |
| `status` | TEXT | One of `pending`, `processing`, `deleted`, `failed`, `expired` |
| `retry_count` | INTEGER | Number of times the message has been nacked |
| `max_retries` | INTEGER | Maximum retries allowed for this message |
| `last_error` | TEXT | Error message from most recent nack |
| `visibility_timeout` | TIMESTAMPTZ | When the message becomes visible again (null until dequeued) |
| `expires_at` | TIMESTAMPTZ | When the message expires (null if no TTL) |
| `original_topic` | TEXT | Original topic if this is a DLQ message; null otherwise |
| `dlq_moved_at` | TIMESTAMPTZ | When the message was promoted to DLQ; null otherwise |
| `created_at`, `updated_at` | TIMESTAMPTZ | Lifecycle timestamps |

**Index**: Composite index on `(topic, status, visibility_timeout, created_at)` for efficient dequeue queries.

### Dequeue Algorithm

1. Query for the oldest pending message in the topic that is either not yet visible or has expired visibility, has not exceeded its retry limit, and has not expired by TTL.
2. Use `FOR UPDATE SKIP LOCKED` to prevent concurrent consumers from acquiring the same message.
3. Transition the message to `'processing'` status and set `visibility_timeout` to `now() + [visibility timeout duration]`.
   - The duration is determined by the optional `visibility_timeout_seconds` field in the `DequeueRequest` (if provided and > 0)
   - Otherwise, the server-wide `visibility_timeout` configuration is used
4. Return the message to the consumer.

### Per-Dequeue Visibility Timeout Override

Clients can override the server-wide visibility timeout on a per-dequeue basis by setting the optional `visibility_timeout_seconds` field in the `DequeueRequest`. This is useful for consumers with variable processing times. For example, a slow batch processor can request a longer timeout without changing the global config. When `visibility_timeout_seconds` is omitted or not set, the server-wide default applies. Setting it to 0 is rejected with an `InvalidArgument` error.

### Message Statuses

- **pending** (yellow badge) — Ready to be dequeued (initial state after enqueue, or reset after a nack with retries remaining, or after requeue from DLQ)
- **processing** (blue badge) — Currently held by a consumer (after dequeue, until ack or nack)
- **deleted** — Acknowledged by consumer; permanently removed from the queue
- **failed** (red badge) — Nacked with no retries remaining (only when DLQ threshold is disabled or message has not reached threshold)
- **expired** (orange badge) — Marked by the expiry reaper after TTL elapsed

### Message Lifecycle

- **pending** → (dequeued) → **processing** → (acknowledged) → **deleted**
- **pending** → (dequeued) → **processing** → (nacked, retries remaining and below DLQ threshold) → **pending** (automatically retried)
- **pending** → (dequeued) → **processing** → (nacked, DLQ threshold reached) → moved to **<topic>.dlq** as **pending** (with max_retries = 0)
- **<topic>.dlq pending** → (manually requeued) → **pending** in original topic (resets retry_count and restores max_retries)
- **pending** or **processing** → (TTL expires) → **expired** (marked by automatic reaper)
- **pending** → (dequeued) → **processing** → (visibility timeout expires) → **pending** (automatically reappears)

### Dead-Letter Queue Details

When a message reaches the DLQ threshold, it is automatically promoted to a separate queue with the topic name `<original-topic>.dlq`. For example, messages from the `orders` topic that exceed the DLQ threshold are moved to `orders.dlq`.

In the DLQ topic:
- The message is stored with `status = 'pending'` and `max_retries = 0`, preventing automatic retries
- `original_topic` is set to the source topic (e.g., `orders`)
- `dlq_moved_at` is set to the promotion timestamp
- `retry_count` resets to 0

To reprocess a DLQ message, call the `POST /api/messages/:id/requeue` endpoint. This restores the message to its original topic with `retry_count = 0` and `max_retries` restored to the configured default, allowing it to be dequeued and processed again.

> **Note:** The DLQ topic name (`<topic>.dlq`) is reserved. Attempting to enqueue directly to a topic ending in `.dlq` returns an `ErrReservedTopic` error.

## Architecture

### Backend

The backend is a Go service with two concurrent servers:

```
cmd/server/main.go
├── gRPC Server (port 50051)
│   └── Handles queue operations (Enqueue, Dequeue, Ack, Nack)
│       └── Requires JWT auth if enabled
│
└── HTTP Server (port 8080)
    ├── /healthz                             GET    Health check
    ├── /api/auth/login                      POST   Authenticate user, return JWT token
    ├── /api/auth/refresh                    POST   Refresh JWT token
    ├── /api/auth/status                     GET    Authentication status
    ├── /api/messages                        GET    List messages (with optional topic filter)
    ├── /api/messages                        POST   Enqueue a message
    ├── /api/messages/:id/nack               POST   Nack a processing message
    ├── /api/messages/:id/requeue            POST   Requeue a DLQ message
    ├── /api/stats                           GET    Queue depth statistics
    ├── /api/topic-configs                   GET    List all topic configurations
    ├── /api/topic-configs/:topic            PUT    Create/update topic configuration
    ├── /api/topic-configs/:topic            DELETE Delete topic configuration
    ├── /api/topic-schemas                   GET    List all registered schemas
    ├── /api/topic-schemas/:topic            PUT    Register or update a schema
    ├── /api/topic-schemas/:topic            DELETE Delete a registered schema
    ├── /api/topic-schemas/:topic            GET    Fetch a single schema
    ├── /api/users                           GET    List all users (admin-only)
    ├── /api/users                           POST   Create new user (admin-only)
    ├── /api/users/:id                       PUT    Update user (admin-only)
    ├── /api/users/:id                       DELETE Delete user (admin-only)
    ├── /api/users/:id/grants                GET    List user grants (admin-only)
    ├── /api/users/:id/grants                POST   Create grant for user (admin-only)
    ├── /api/users/:id/grants/:grant_id      DELETE Delete user grant (admin-only)
    ├── /metrics                             GET    Prometheus metrics (unauthenticated)
    └── /api/* endpoints require JWT auth if enabled; /metrics is unauthenticated
```

Both servers connect to the same `queue.Service` instance, which manages all message operations against PostgreSQL.

### Backend Layers

```
internal/
├── config/          Configuration loading (Viper YAML + env vars)
├── db/              PostgreSQL connectivity and migrations
├── queue/           Core queue logic (Service, Message types)
├── server/          gRPC and HTTP server implementations
├── auth/            JWT and basic auth handling
└── pb/              Generated protobuf Go bindings (read-only)
```

### Frontend

The admin UI is an Angular Single Page Application (Nx workspace) that communicates exclusively with the HTTP API on port 8080.

```
admin-ui/src/app/
├── services/
│   ├── queue.service.ts         HTTP client (GET /api/messages, POST /api/messages, POST /api/messages/:id/nack, POST /api/messages/:id/requeue)
│   └── auth.service.ts          Manages login state and credentials
├── interceptors/
│   └── auth.interceptor.ts      Injects Authorization header on all requests
├── guards/
│   └── auth.guard.ts            Protects routes; redirects to login if unauthorized
├── login/                        Authentication UI; stores credentials in localStorage
├── messages/                     Message list with status badges, retry/expiry columns, DLQ highlighting, and inline Nack/Requeue actions
└── services/                     Shared HTTP and auth services
```

**Admin UI Features**:
- **Message table** — Displays all messages with ID, topic, payload, status badge, retry count, expiry time, and metadata
- **Status badges** — Color-coded visual indicators: `pending` (yellow), `processing` (blue), `failed` (red), `expired` (orange)
- **Retry information** — Shows `retry_count / max_retries` with a tooltip displaying `last_error` when available
- **DLQ highlighting** — Dead-letter queue messages (`<topic>.dlq`) are highlighted with an amber background and show the original topic as a sub-label
- **Requeue action** — DLQ messages display a "Requeue" button to move them back to their original topic
- **Inline Nack** — Processing messages display a "Nack" button that expands an inline text input for an optional error reason
- **Topic filtering** — Filter the message list by topic name
- **Manual enqueue** — Form to enqueue test messages with topic, payload (JSON), and optional metadata key-value pairs
- **Config tab** — Interactive editor for per-topic configuration overrides without server restart

**Note**: The gRPC server (port 50051) is for queue client applications only; the UI uses HTTP exclusively.

## API Reference

### gRPC Service

The gRPC service implements the `QueueService` defined in `proto/queue.proto`. All methods require JWT auth if enabled.

#### Enqueue

Enqueues a message to a topic.

```protobuf
rpc Enqueue(EnqueueRequest) returns (EnqueueResponse);

message EnqueueRequest {
  string topic = 1;                    // Topic name (required)
  bytes payload = 2;                   // Message payload (required)
  map<string, string> metadata = 3;    // Optional metadata
}

message EnqueueResponse {
  string id = 1;  // UUID of the enqueued message
}
```

#### Dequeue

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
}
```

Returns `codes.NotFound` if no messages are available; otherwise returns the next message and transitions it to `'processing'` status with a visibility timeout.

**Visibility Timeout Behavior**:
- When `visibility_timeout_seconds` is **omitted or not set**, the server-wide `visibility_timeout` configuration is used (default 30 seconds).
- When `visibility_timeout_seconds` is **set to a value > 0**, that duration (in seconds) overrides the server-wide configuration for this dequeue operation only.
- When `visibility_timeout_seconds` is **set to 0**, the request is rejected with `codes.InvalidArgument`.

#### Ack

Acknowledges (deletes) a processing message.

```protobuf
rpc Ack(AckRequest) returns (AckResponse);

message AckRequest {
  string id = 1;  // Message UUID (required)
}

message AckResponse {}
```

Fails if the message is not found or not in `'processing'` status.

#### Nack

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

### HTTP Admin API

All HTTP endpoints are authenticated via JWT if enabled.

#### GET /healthz

Health check endpoint. Always returns 200 OK.

```bash
curl http://localhost:8080/healthz
```

#### GET /api/messages

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
    "status": "pending",
    "retry_count": 0,
    "max_retries": 3,
    "created_at": "2025-04-25T12:00:00Z"
  }
]
```

#### POST /api/messages

Enqueues a message.

```bash
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "orders",
    "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
    "metadata": {"user_id": "42"}
  }'
```

**Response:** HTTP 201 Created
```json
{"id": "550e8400-e29b-41d4-a716-446655440000"}
```

#### POST /api/messages/:id/nack

Signals that processing of a message failed.

```bash
curl -X POST http://localhost:8080/api/messages/:id/nack \
  -H "Content-Type: application/json" \
  -d '{"error": "connection timeout"}'
```

The `error` field is optional. If provided, it is stored in the message's `last_error` field.

**Response:** HTTP 204 No Content on success.

**Behavior**: If the message has retries remaining and has not reached the DLQ threshold, its status reverts to `'pending'` and it can be dequeued again. If the DLQ threshold is reached, the message is promoted to the dead-letter queue. Otherwise, its status becomes `'failed'`.

#### POST /api/messages/:id/requeue

Moves a dead-letter queue message back to its original topic for reprocessing.

```bash
curl -X POST http://localhost:8080/api/messages/:id/requeue
```

**Response:** HTTP 204 No Content on success.

**Behavior**: Restores the message to its original topic (retrieved from `original_topic`), resets `retry_count` to 0, restores `max_retries` to the configured default, and sets status to `'pending'`.

Returns HTTP 404 if the message is not found or is not a dead-letter message.

#### GET /api/topic-configs

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
      "max_depth": 1000
    }
  ]
}
```

#### PUT /api/topic-configs/:topic

Creates or updates a topic-level configuration. Omitting a field or sending `null` reverts that setting to the global default.

```bash
curl -u admin:secret -X PUT http://localhost:8080/api/topic-configs/orders \
  -H "Content-Type: application/json" \
  -d '{
    "max_retries": 5,
    "message_ttl_seconds": 3600,
    "max_depth": 1000
  }'
```

**Response:** HTTP 200 OK

#### DELETE /api/topic-configs/:topic

Deletes a topic-level configuration, reverting all settings to global defaults.

```bash
curl -X DELETE -u admin:secret http://localhost:8080/api/topic-configs/orders
```

**Response:** HTTP 204 No Content

#### GET /api/stats

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

## Observability

### Prometheus Metrics

queue-ti exposes Prometheus metrics on the HTTP server at the `/metrics` endpoint (port 8080) in Prometheus text format. Metrics are exported in real time and require no additional configuration.

> **Note**: The `/metrics` endpoint is **unauthenticated** even when `auth.enabled: true`. This is by design — operators typically protect this endpoint at the network or reverse proxy level.

#### Metrics Endpoint

```bash
GET http://localhost:8080/metrics
```

#### Prometheus Scrape Configuration

Add this to your Prometheus configuration (`prometheus.yml`):

```yaml
scrape_configs:
  - job_name: queue-ti
    static_configs:
      - targets: ['localhost:8080']
    scrape_interval: 15s
```

#### Exported Metrics

**Counters** (cumulative, monotonically increasing):

| Metric | Labels | Description |
|--------|--------|-------------|
| `queueti_enqueued_total` | `topic` | Total messages enqueued |
| `queueti_dequeued_total` | `topic` | Total messages dequeued |
| `queueti_acked_total` | `topic` | Total messages acknowledged (deleted) |
| `queueti_nacked_total` | `topic`, `outcome` | Total messages nacked; outcome: `retry`, `failed`, or `dlq` |
| `queueti_requeued_total` | `topic` | Total messages requeued from DLQ to original topic |
| `queueti_expired_total` | — | Total messages expired by the automatic reaper |

**Gauge** (sampled from database on each scrape):

| Metric | Labels | Description |
|--------|--------|-------------|
| `queueti_queue_depth` | `topic`, `status` | Current number of messages per topic and status |

#### Example Scrape Output

```
# HELP queueti_enqueued_total Total messages enqueued
# TYPE queueti_enqueued_total counter
queueti_enqueued_total{topic="orders"} 1042
queueti_enqueued_total{topic="payments"} 523

# HELP queueti_queue_depth Number of messages per topic and status
# TYPE queueti_queue_depth gauge
queueti_queue_depth{status="pending",topic="orders"} 5
queueti_queue_depth{status="processing",topic="orders"} 2
queueti_queue_depth{status="deleted",topic="orders"} 1028
```

#### Recommended Alerts

Consider setting up these Prometheus alerts for production deployments:

```yaml
groups:
  - name: queue-ti
    rules:
      # Alert if queue depth grows unbounded
      - alert: QueueTIHighQueueDepth
        expr: queueti_queue_depth{status="pending"} > 1000
        for: 5m
        annotations:
          summary: "High queue depth on {{ $labels.topic }}"

      # Alert on high nack rate (potential consumer issue)
      - alert: QueueTIHighNackRate
        expr: rate(queueti_nacked_total[5m]) > 10
        for: 5m
        annotations:
          summary: "High nack rate on {{ $labels.topic }}"

      # Alert if DLQ is accumulating messages
      - alert: QueueTIHighDLQPromotion
        expr: increase(queueti_nacked_total{outcome="dlq"}[1h]) > 50
        for: 5m
        annotations:
          summary: "DLQ accumulation on {{ $labels.topic }}"
```

## Running Tests

### Backend Tests (Go, Ginkgo)

Run all tests:
```bash
make test
```

Run a specific package:
```bash
ginkgo ./internal/queue/...
```

Tests use TestContainers to spin up a real PostgreSQL instance; no mocking of the database.

### Frontend Tests (Angular, Vitest)

```bash
cd admin-ui
npx nx test
```

### Test Coverage

Check coverage for the backend:
```bash
ginkgo ./... -cover
```

## Performance Testing

### Go Benchmarks

The queue package includes benchmarks that exercise the core queue operations directly against a real PostgreSQL instance (spun up via TestContainers).

```bash
# Run all benchmarks, 3 seconds per benchmark
go test -bench=. -benchtime=3s -run=^$ ./internal/queue/...

# Run a specific benchmark
go test -bench=BenchmarkEnqueue -benchtime=5s -run=^$ ./internal/queue/...

# Include memory allocation stats
go test -bench=. -benchmem -run=^$ ./internal/queue/...
```

Available benchmarks:

| Benchmark | What it measures |
|---|---|
| `BenchmarkEnqueue` | Sequential single-goroutine enqueue throughput |
| `BenchmarkEnqueueParallel` | Concurrent enqueue across `GOMAXPROCS` goroutines |
| `BenchmarkDequeueAck` | Dequeue + Ack hot path (pre-seeded queue, no enqueue overhead) |
| `BenchmarkFullPipeline` | Full Enqueue → Dequeue → Ack round-trip under parallel load |

Example output:
```
BenchmarkEnqueue-10               3106   1.15ms/op
BenchmarkEnqueueParallel-10      18234   192µs/op
BenchmarkDequeueAck-10            4821   621µs/op
BenchmarkFullPipeline-10          9344   320µs/op
```

### End-to-End Load Test

The `cmd/loadtest` binary connects to a running gRPC server and drives configurable numbers of concurrent producers and consumers.

**Start the stack first:**
```bash
docker-compose up
```

**Run the load test:**
```bash
go run ./cmd/loadtest [flags]
```

Available flags:

| Flag | Default | Description |
|---|---|---|
| `--addr` | `localhost:50051` | gRPC server address |
| `--duration` | `30s` | How long to run |
| `--producers` | `4` | Concurrent enqueue goroutines |
| `--consumers` | `4` | Concurrent dequeue+ack goroutines |
| `--topic` | `loadtest` | Topic to use |
| `--msg-size` | `256` | Payload size in bytes |
| `--token` | _(empty)_ | Bearer JWT for authenticated servers |

**Examples:**

```bash
# Default: 4 producers, 4 consumers, 30 seconds
go run ./cmd/loadtest

# High concurrency, 2-minute run
go run ./cmd/loadtest --producers=16 --consumers=16 --duration=2m

# Authenticated server
go run ./cmd/loadtest --token=<jwt>

# Large payloads (1 KB), longer run
go run ./cmd/loadtest --msg-size=1024 --duration=60s
```

Progress is printed to stderr every 5 seconds; the final summary goes to stdout:

```
[5s] enqueue: 7,503 | dequeue+ack: 7,441
[10s] enqueue: 15,021 | dequeue+ack: 14,899
...

=== Load Test Results (30s, 4 producers, 4 consumers) ===

Enqueue
  Total:      45,210 ops
  Throughput: 1,507 ops/s
  p50:        2.1ms
  p95:        5.8ms
  p99:        12.3ms
  Errors:     0

Dequeue+Ack
  Total:      44,987 ops
  Throughput: 1,499 ops/s
  p50:        3.4ms
  p95:        8.1ms
  p99:        18.2ms
  Errors:     0
```

#### Running with Authentication

When `auth.enabled = true`, obtain a token first via the HTTP login endpoint, then pass it to the load test:

```bash
# Log in and capture the token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' \
  | jq -r '.token')

# Run the load test with the token
go run ./cmd/loadtest --token=$TOKEN
```

Or with the Makefile target:
```bash
make bench-loadtest LOADTEST_FLAGS="--token=$TOKEN --producers=8 --consumers=8 --duration=60s"
```

The token is valid for 15 minutes. For longer runs, obtain a fresh token or use `POST /api/auth/refresh`:
```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/refresh \
  -H "Authorization: Bearer $TOKEN" \
  | jq -r '.token')
```

## Development Workflow

### Regenerating Protobuf

After modifying `proto/queue.proto`:

```bash
make proto
```

This regenerates `internal/pb/queue.pb.go` and `internal/pb/queue_grpc.pb.go`. **Never hand-edit files in `pb/`.**

### Dependency Management

Update Go dependencies:
```bash
make deps
```

Update frontend dependencies:
```bash
cd admin-ui
npm update
```

## Project Structure

```
queue-ti/
├── Makefile                 Make targets for backend build/test/proto
├── Dockerfile               Containerizes the backend
├── docker-compose.yaml      Multi-container setup (PostgreSQL + backend + frontend)
├── config.yaml              Default configuration (overridable via env vars)
├── go.mod, go.sum           Go module definition (root module)
├── go.work                  Go workspace — includes root and client/ modules
├── client/                  Go client library (separate module)
│   ├── go.mod               Module: github.com/Joessst-Dev/queue-ti/client
│   ├── client.go            Dial, NewProducer, NewConsumer
│   ├── producer.go          Producer.Publish
│   ├── consumer.go          Consumer.Consume with auto-reconnect
│   ├── message.go           Message type with Ack/Nack methods
│   ├── options.go           Dial and consumer functional options
│   └── README.md            Full library documentation
├── proto/
│   └── queue.proto          gRPC service definition
├── pb/                      Generated protobuf Go bindings (read-only)
├── cmd/
│   └── server/
│       └── main.go          Server entry point
├── internal/
│   ├── config/              Configuration loading
│   ├── db/
│   │   ├── postgres.go       PostgreSQL connection and migration runner
│   │   └── migrations/       SQL migration files (golang-migrate)
│   ├── queue/
│   │   └── service.go        Core queue logic
│   ├── server/
│   │   ├── grpc.go           gRPC server implementation
│   │   └── http.go           HTTP server implementation
│   └── auth/
│       └── interceptor.go    JWT auth handling
├── admin-ui/                Angular SPA (Nx workspace)
│   ├── package.json
│   ├── nx.json
│   └── src/app/
│       ├── services/        HTTP client and auth services
│       ├── interceptors/    Request/response interceptors
│       ├── guards/          Route guards
│       ├── login/           Login component
│       └── messages/        Message list and detail components
└── README.md
```

## Deployment

### Docker

Build the Docker image:
```bash
docker build -t queue-ti:latest .
```

Run with Docker:
```bash
docker run -d \
  -p 50051:50051 \
  -p 8080:8080 \
  -e QUEUETI_DB_HOST=postgres \
  -e QUEUETI_DB_USER=postgres \
  -e QUEUETI_DB_PASSWORD=postgres \
  -e QUEUETI_DB_NAME=queueti \
  queue-ti:latest
```

### Docker Compose

The included `docker-compose.yaml` orchestrates PostgreSQL, the backend, and the admin UI:

```bash
docker-compose up -d
```

Access the admin UI at `http://localhost:8081` (login: `admin` / `secret`).

## Release Management

### Versioning

queue-ti uses [Semantic Versioning](https://semver.org). A single `v1.2.3` tag on `main` drives all release artifacts:

| Artifact | Published as |
|---|---|
| Docker image | `ghcr.io/<owner>/queue-ti:v1.2.3` and `:latest` on GHCR |
| Go client library | `github.com/Joessst-Dev/queue-ti/client@v1.2.3` (sub-module tag `client/v1.2.3`) |
| GitHub Release | Auto-generated changelog with Docker pull and `go get` commands |

### Cutting a Release

1. Ensure `main` is in a releasable state — CI must be green.
2. Push a version tag:
   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```
3. The release pipeline runs automatically and will:
   - Run the full backend and frontend test suites (release is blocked on failure)
   - Build and push a multi-arch Docker image (`linux/amd64` + `linux/arm64`) to GHCR
   - Create the `client/v1.2.3` Go sub-module tag on the same commit
   - Publish a GitHub Release with auto-generated notes

Monitor the run at **Actions → Release** in the GitHub repository.

### Using a Release

**Docker:**
```bash
docker pull ghcr.io/<owner>/queue-ti:v1.2.3
```

Or with docker-compose, pin the image tag in `docker-compose.yaml`:
```yaml
services:
  queueti:
    image: ghcr.io/<owner>/queue-ti:v1.2.3
```

**Go client library:**
```bash
go get github.com/Joessst-Dev/queue-ti/client@v1.2.3
```

### CI Pipeline

The CI pipeline (`.github/workflows/ci.yml`) runs on every push and pull request:

| Job | What it does |
|---|---|
| `backend` | `go build`, Ginkgo test suite with a real PostgreSQL container |
| `frontend` | Angular production build, Vitest unit tests, ESLint |
| `build-image` | Docker image build (no push) — catches Dockerfile regressions early |

The release pipeline (`.github/workflows/release.yml`) runs only on `v*.*.*` tag pushes and reuses the same test jobs as a gate before publishing any artifact.

### Changelog

Release notes are generated automatically by GitHub from merged PR titles and commit messages since the previous tag. To produce meaningful changelogs, use descriptive PR titles and squash-merge feature branches.

## Troubleshooting

### gRPC connection refused on port 50051

Check that the backend is running:
```bash
make run
```

If using Docker Compose, verify the service is healthy:
```bash
docker-compose ps
```

### HTTP 401 Unauthorized

If authentication is enabled (`QUEUETI_AUTH_ENABLED=true`), ensure you are providing valid JWT credentials:
```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/messages
```

Check the current auth status:
```bash
curl http://localhost:8080/api/auth/status
```

### PostgreSQL connection errors

Verify the PostgreSQL service is running and the credentials in `config.yaml` or environment variables are correct:

```bash
# Test connection with psql
psql -h localhost -U postgres -d queueti -c "SELECT 1;"
```

## Contributing

To contribute to queue-ti:

1. Create a feature branch
2. Make your changes
3. Run tests: `make test` (backend) and `cd admin-ui && npx nx test` (frontend)
4. Regenerate protobuf if needed: `make proto`
5. Submit a pull request

## License

MIT
