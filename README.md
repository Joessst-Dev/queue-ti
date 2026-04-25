# queue-ti

A distributed message queue service built with Go gRPC and PostgreSQL, with an Angular admin UI for queue management and message inspection.

## Features

- **gRPC API** — High-performance queue operations (enqueue, dequeue, acknowledge) over gRPC
- **HTTP Admin API** — REST endpoints for queue inspection and management from the admin UI
- **Topic-based routing** — Messages are organized by topic; multiple independent queues share a single PostgreSQL table
- **At-least-once delivery** — Messages are guaranteed to be delivered at least once via configurable visibility timeout (default 30 seconds)
- **Contention-free dequeue** — Uses `FOR UPDATE SKIP LOCKED` for lock-free concurrent message consumption
- **Basic authentication** — Optional basic auth for both gRPC and HTTP endpoints
- **Admin UI** — Angular-based web interface to list messages, filter by topic, and manually enqueue test messages
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
  - `status` (TEXT, default `'pending'`)
  - `visibility_timeout` (TIMESTAMPTZ, null until dequeue)
  - `created_at`, `updated_at` (TIMESTAMPTZ)

- **Index**: Composite index on `(topic, status, visibility_timeout, created_at)` for efficient dequeue queries.

- **Dequeue algorithm**:
  1. Query for the oldest pending message in the topic that is either not yet visible or has expired.
  2. Use `FOR UPDATE SKIP LOCKED` to prevent concurrent consumers from acquiring the same message.
  3. Transition the message to `'processing'` status and set `visibility_timeout` to `now() + [configured duration]`.
  4. Return the message to the consumer.

- **Message lifecycle**:
  - **pending** → (dequeued) → **processing** → (acknowledged) → **deleted**
  - **pending** → (dequeued) → **processing** → (visibility timeout expires) → **pending** (automatically reappears)

### Frontend

The admin UI is an Angular Single Page Application (Nx workspace) that communicates exclusively with the HTTP API on port 8080.

```
admin-ui/src/app/
├── services/
│   ├── queue.service.ts         HTTP client (GET /api/messages, POST /api/messages)
│   └── auth.service.ts          Manages login state and credentials
├── interceptors/
│   └── auth.interceptor.ts      Injects Authorization header on all requests
├── guards/
│   └── auth.guard.ts            Protects routes; redirects to login if unauthorized
├── login/                        Authentication UI; stores credentials in localStorage
├── messages/                     Message list and detail views
└── services/                     Shared HTTP and auth services
```

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
    "created_at": "2025-04-25T12:00:00Z"
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
- **No message expiration** — Messages remain in the queue indefinitely unless manually deleted or acknowledged.
- **Single-table design** — All topics share one PostgreSQL table; consider partitioning for very high throughput.
- **No dead-letter queue** — Messages are not moved to a separate queue after repeated dequeue failures.
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
