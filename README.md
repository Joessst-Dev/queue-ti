# queue-ti

A distributed message queue service built with Go gRPC and PostgreSQL, with an Angular admin UI for queue management and message inspection.

## Features

- **gRPC API** — High-performance queue operations (enqueue, dequeue, acknowledge, nack) over gRPC
- **HTTP Admin API** — REST endpoints for queue inspection and management from the admin UI
- **Topic-based routing** — Messages are organized by topic; multiple independent queues share a single PostgreSQL table
- **At-least-once delivery** — Messages are guaranteed to be delivered at least once via configurable visibility timeout (default 30 seconds)
- **Automatic retries** — Failed messages are automatically retried up to a configurable limit (default 3 retries); consumers call `Nack` to signal failure
- **Dead-letter queue (DLQ)** — Messages that exhaust their retry limit are automatically promoted to a `<topic>.dlq` topic; can be manually requeued back to the original topic
- **Message TTL** — Messages can expire after a configurable duration (default 24 hours); an automatic reaper marks expired messages
- **Contention-free dequeue** — Uses `FOR UPDATE SKIP LOCKED` for lock-free concurrent message consumption
- **Basic authentication** — Optional basic auth for both gRPC and HTTP endpoints
- **Admin UI** — Angular-based web interface to list messages with detailed status, retry counts, and expiry information; filter by topic; manually enqueue test messages; inspect dead-letter queue messages; requeue or inline-nack processing messages
- **Configuration** — YAML-based configuration with environment variable overrides via `QUEUETI_` prefix

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
   The server will listen on:
   - gRPC: `localhost:50051` (for queue producers/consumers)
   - HTTP: `localhost:8080` (for admin UI and REST API)

4. **Start the admin UI** (in another terminal)
   ```bash
   cd admin-ui
   npm install
   npx nx serve
   ```
   The UI will be available at `http://localhost:4200`

5. **Clean up PostgreSQL**
   ```bash
   docker stop queueti-postgres
   ```

### Docker Compose

Deploy the full stack (PostgreSQL + backend + admin UI) with one command:

```bash
docker-compose up
```

The admin UI will be available at `http://localhost:8081` (username: `admin`, password: `secret`).

## Configuration

Configuration can be provided via `config.yaml` at the repository root or overridden with environment variables prefixed `QUEUETI_`.

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
  visibility_timeout: 30s  # Time a dequeued message remains invisible to other consumers
  max_retries: 3           # Maximum number of retries for a failed message
  message_ttl: 24h         # Time-to-live for messages (0 = no expiry)
  dlq_threshold: 3         # Retry count at which messages are promoted to DLQ (0 = disabled)

auth:
  enabled: false
  username: admin
  password: secret
```

### Environment Variables

Any configuration key can be overridden with an environment variable. Use the key path with underscores and the `QUEUETI_` prefix:

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
| `QUEUETI_AUTH_ENABLED` | Enable authentication | `true` |
| `QUEUETI_AUTH_USERNAME` | Basic auth username | `admin` |
| `QUEUETI_AUTH_PASSWORD` | Basic auth password | `secret` |

## Architecture

### Backend

The backend is a Go service with two concurrent servers:

```
cmd/server/main.go
├── gRPC Server (port 50051)
│   └── Handles queue operations (Enqueue, Dequeue, Ack)
│       └── Requires basic auth if enabled
│
└── HTTP Server (port 8080)
    ├── /healthz                     GET    Health check
    ├── /api/auth/status             GET    Authentication status
    ├── /api/messages                GET    List messages (with optional topic filter)
    └── /api/messages                POST   Enqueue a message
        └── Requires basic auth if enabled
```

Both servers connect to the same `queue.Service` instance, which manages all message operations against PostgreSQL.

#### Backend Layers

```
internal/
├── config/          Configuration loading (Viper YAML + env vars)
├── db/              PostgreSQL connectivity and migrations
├── queue/           Core queue logic (Service, Message types)
├── server/          gRPC and HTTP server implementations
├── auth/            Basic auth interceptor for gRPC
└── pb/              Generated protobuf Go bindings (read-only)
```

### Queue Mechanics

- **Data model**: Single `messages` PostgreSQL table with columns:
  - `id` (UUID, primary key)
  - `topic` (TEXT, required)
  - `payload` (BYTEA, required)
  - `metadata` (JSONB, optional)
  - `status` (TEXT, one of `pending`, `processing`, `deleted`, `failed`, `expired`)
  - `retry_count` (INTEGER, incremented on each nack)
  - `max_retries` (INTEGER, set at enqueue time; set to 0 for DLQ messages)
  - `last_error` (TEXT, error message from most recent nack)
  - `visibility_timeout` (TIMESTAMPTZ, null until dequeue)
  - `expires_at` (TIMESTAMPTZ, calculated at enqueue based on TTL)
  - `original_topic` (TEXT, set when message is promoted to DLQ; null for regular messages)
  - `dlq_moved_at` (TIMESTAMPTZ, set when message is promoted to DLQ; null otherwise)
  - `created_at`, `updated_at` (TIMESTAMPTZ)

- **Index**: Composite index on `(topic, status, visibility_timeout, created_at)` for efficient dequeue queries.

- **Dequeue algorithm**:
  1. Query for the oldest pending message in the topic that is either not yet visible or has expired, has not exceeded its retry limit, and has not expired by TTL.
  2. Use `FOR UPDATE SKIP LOCKED` to prevent concurrent consumers from acquiring the same message.
  3. Transition the message to `'processing'` status and set `visibility_timeout` to `now() + [configured duration]`.
  4. Return the message to the consumer.

- **Message statuses**:
  - **pending** (yellow badge) — Ready to be dequeued (initial state after enqueue, or reset after a nack with retries remaining, or after requeue from DLQ)
  - **processing** (blue badge) — Currently held by a consumer (after dequeue, until ack or nack)
  - **deleted** — Acknowledged by consumer; permanently removed from the queue
  - **failed** (red badge) — Nacked with no retries remaining (only when DLQ threshold is disabled or message has not reached threshold)
  - **expired** (orange badge) — Marked by the expiry reaper after TTL elapsed

- **Message lifecycle**:
  - **pending** → (dequeued) → **processing** → (acknowledged) → **deleted**
  - **pending** → (dequeued) → **processing** → (nacked, retries remaining and below DLQ threshold) → **pending** (automatically retried)
  - **pending** → (dequeued) → **processing** → (nacked, DLQ threshold reached) → moved to **<topic>.dlq** as **pending** (with max_retries = 0)
  - **<topic>.dlq pending** → (manually requeued) → **pending** in original topic (resets retry_count and restores max_retries)
  - **pending** or **processing** → (TTL expires) → **expired** (marked by automatic reaper)
  - **pending** → (dequeued) → **processing** → (visibility timeout expires) → **pending** (automatically reappears)

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
- **Inline Nack** — Processing messages display a "Nack" button that expands an inline text input for an optional error reason; confirmation triggers the nack operation
- **Topic filtering** — Filter the message list by topic name
- **Manual enqueue** — Form to enqueue test messages with topic, payload (JSON), and optional metadata key-value pairs

**Note**: The gRPC server (port 50051) is for queue client applications only; the UI uses HTTP exclusively.

## API Reference

### gRPC Service

The gRPC service implements the `QueueService` defined in `proto/queue.proto`. All methods require basic authentication if enabled.

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
  string topic = 1;  // Topic name (required)
}

message DequeueResponse {
  string id = 1;                        // Message UUID
  string topic = 2;                     // Topic name
  bytes payload = 3;                    // Message payload
  map<string, string> metadata = 4;     // Metadata
  google.protobuf.Timestamp created_at = 5;  // Creation timestamp
}
```

Returns `codes.NotFound` if no messages are available; otherwise returns the next message and transitions it to `'processing'` status with a visibility timeout.

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
- If `retry_count + 1 >= dlq_threshold` (and `dlq_threshold > 0`), the message is **promoted to the dead-letter queue** (`<topic>.dlq`). Its `original_topic` is recorded, `max_retries` is set to 0 (preventing auto-retry), `retry_count` resets to 0, and status becomes `'pending'` in the DLQ topic.
- Otherwise, if `retry_count + 1 < max_retries`, its status reverts to `'pending'` and `retry_count` is incremented.
- Otherwise, its status becomes `'failed'`.

Fails if the message is not found or not in `'processing'` status.

### HTTP Admin API

All HTTP endpoints are authenticated via basic auth if enabled.

#### GET /healthz

Health check endpoint. Always returns 200 OK.

```bash
curl http://localhost:8080/healthz
```

#### GET /api/auth/status

Returns whether authentication is required.

```bash
curl http://localhost:8080/api/auth/status
# {"auth_required": false}
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
  },
  {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "topic": "orders.dlq",
    "payload": "eyJvcmRlcl9pZCI6IDU2Nzg5fQ==",
    "metadata": {"user_id": "43"},
    "status": "pending",
    "retry_count": 0,
    "max_retries": 0,
    "original_topic": "orders",
    "dlq_moved_at": "2025-04-25T12:05:00Z",
    "created_at": "2025-04-25T12:04:00Z"
  }
]
```

#### POST /api/messages

Enqueues a message.

**Request body:**
```json
{
  "topic": "orders",
  "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ==",
  "metadata": {"user_id": "42"}
}
```

**Response:** HTTP 201 Created with the message ID.

```json
{"id": "550e8400-e29b-41d4-a716-446655440000"}
```

#### POST /api/messages/:id/nack

Signals that processing of a message failed.

**Request body:**
```json
{
  "error": "connection timeout"
}
```

The `error` field is optional; if provided, it is stored in the message's `last_error` field.

**Response:** HTTP 204 No Content on success.

**Behavior**: If the message has retries remaining and has not reached the DLQ threshold, its status reverts to `'pending'` and it can be dequeued again. If the DLQ threshold is reached, the message is promoted to the dead-letter queue (`<topic>.dlq`). Otherwise, its status becomes `'failed'`.

#### POST /api/messages/:id/requeue

Moves a dead-letter queue message back to its original topic for reprocessing.

**Request body:** Empty (no body required)

**Response:** HTTP 204 No Content on success.

**Behavior**: Restores the message to its original topic (retrieved from `original_topic`), resets `retry_count` to 0, restores `max_retries` to the configured default, and sets status to `'pending'`. The message can then be dequeued and processed again.

Returns HTTP 404 if the message is not found or is not a dead-letter message (i.e., does not have `original_topic` set).

### Authentication

When `auth.enabled: true`, both gRPC and HTTP endpoints require HTTP Basic Authentication.

**gRPC**: Include the `authorization` metadata header:
```
authorization: Basic <base64(username:password)>
```

**HTTP**: Include the `Authorization` header:
```
Authorization: Basic <base64(username:password)>
```

Example with curl:
```bash
curl -u admin:secret http://localhost:8080/api/messages
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
├── go.mod, go.sum           Go module definition
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
│       └── interceptor.go    Basic auth interceptor
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

Access the admin UI at `http://localhost:8081` (admin / secret).

## Automatic Expiry and Retry Management

### Expiry Reaper

When `message_ttl` is greater than zero, a background goroutine starts automatically on server startup. This reaper runs every 60 seconds and marks any messages with `expires_at < now()` (and status `'pending'` or `'processing'`) as `'expired'`. Expired messages are no longer dequeued by new consumers.

Enable or configure the TTL with:
```bash
QUEUETI_QUEUE_MESSAGE_TTL=24h   # 24 hours; can be 0 to disable
```

### Retry Behavior and Dead-Letter Queue

Every message carries `retry_count` and `max_retries`:
- `max_retries` is set at enqueue time (from `QUEUETI_QUEUE_MAX_RETRIES`, default 3)
- `retry_count` increments each time `Nack` is called
- When `retry_count + 1 >= dlq_threshold` (if `dlq_threshold > 0`), the message is promoted to the dead-letter queue instead of being retried further
- When DLQ is disabled (`dlq_threshold = 0`), messages with `retry_count + 1 >= max_retries` are marked as `'failed'`
- Failed messages are not dequeued

To adjust retry and DLQ settings globally:
```bash
QUEUETI_QUEUE_MAX_RETRIES=5  # Retry up to 5 times (note: DLQ threshold may trigger first)
QUEUETI_QUEUE_DLQ_THRESHOLD=3  # Promote to DLQ after 3 failed nacks
```

**Dead-Letter Queue Details:**

When a message reaches the DLQ threshold, it is automatically promoted to a separate queue with the topic name `<original-topic>.dlq`. For example, messages from the `orders` topic that exceed the DLQ threshold are moved to `orders.dlq`.

In the DLQ topic:
- The message is stored with `status = 'pending'` and `max_retries = 0`, preventing automatic retries
- `original_topic` is set to the source topic (e.g., `orders`)
- `dlq_moved_at` is set to the promotion timestamp
- `retry_count` resets to 0

To reprocess a DLQ message, call the `POST /api/messages/:id/requeue` endpoint. This restores the message to its original topic with `retry_count = 0` and `max_retries` restored to the configured default, allowing it to be dequeued and processed again.

> **Note:** The DLQ topic name (`<topic>.dlq`) is reserved. Attempting to enqueue directly to a topic ending in `.dlq` returns an `ErrReservedTopic` error.

## Environment Variables (Docker Compose Example)

See the "Configuration" section above. Docker Compose sets these in `docker-compose.yaml`:

```yaml
environment:
  QUEUETI_DB_HOST: postgres
  QUEUETI_DB_PORT: "5432"
  QUEUETI_DB_USER: postgres
  QUEUETI_DB_PASSWORD: postgres
  QUEUETI_DB_NAME: queueti
  QUEUETI_DB_SSLMODE: disable
  QUEUETI_SERVER_PORT: "50051"
  QUEUETI_SERVER_HTTP_PORT: "8080"
  QUEUETI_AUTH_ENABLED: "true"
  QUEUETI_AUTH_USERNAME: "admin"
  QUEUETI_AUTH_PASSWORD: "secret"
```

## Known Limitations

- **No priority queues** — Messages are processed in FIFO order by topic.
- **Single-table design** — All topics share one PostgreSQL table; consider partitioning for very high throughput.
- **No message scheduling** — Messages are available for dequeue immediately upon enqueue.

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

If authentication is enabled (`QUEUETI_AUTH_ENABLED=true`), ensure you are providing basic auth credentials:
```bash
curl -u admin:secret http://localhost:8080/api/messages
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
