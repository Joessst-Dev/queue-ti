# Getting Started

## Prerequisites

- Go 1.25.5 or later
- PostgreSQL 16+
- Node.js 20+ (for admin UI development)
- Docker and Docker Compose (optional, for containerized deployment)

## Local Development

### 1. Clone the repository

```bash
git clone https://github.com/Joessst-Dev/queue-ti
cd queue-ti
```

### 2. Set up PostgreSQL

Using Docker:

```bash
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

### 3. Start the backend server

```bash
make run
```

The server listens on:
- **gRPC**: `localhost:50051` (for queue producers/consumers)
- **HTTP**: `localhost:8080` (for admin UI and REST API)

### 4. Start the admin UI (in another terminal)

```bash
cd ui
npm install
npx nx serve
```

The UI is available at `http://localhost:4200`

### 5. Clean up

```bash
docker stop queueti-postgres
```

## Docker Compose

Deploy the full stack (PostgreSQL + backend + admin UI) with one command:

```bash
docker-compose up
```

The admin UI is available at `http://localhost:8081` (login: `admin` / `secret`).

## Your First Message

### Enqueue a message

```bash
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "hello",
    "payload": "eyJtZXNzYWdlIjogIkhyIH0="}
```

The `payload` field is base64-encoded JSON. Decode the example above:
```bash
echo "eyJtZXNzYWdlIjogIkhybGRvIH0=" | base64 -d
# Output: {"message": "Hello"}
```

**Response:**
```json
{"id": "550e8400-e29b-41d4-a716-446655440000"}
```

### View the message in the admin UI

1. Open `http://localhost:4200` (or `http://localhost:8081` if using Docker Compose)
2. Log in with username `admin` and password `secret`
3. You'll see your message in the Messages table with status `pending`

### Dequeue and acknowledge the message

```bash
# Dequeue the message
curl -X POST http://localhost:8080/api/messages/dequeue \
  -H "Content-Type: application/json" \
  -d '{"topic": "hello", "count": 1}'
```

**Response:**
```json
{
  "messages": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "topic": "hello",
      "payload": "eyJtZXNzYWdlIjogIkhlbGxvIn0=",
      "metadata": {},
      "key": null,
      "created_at": "2025-04-25T12:00:00Z",
      "retry_count": 0,
      "max_retries": 3
    }
  ]
}
```

The message status transitions from `pending` to `processing`.

Acknowledge it:
```bash
curl -X POST http://localhost:8080/api/messages/550e8400-e29b-41d4-a716-446655440000/ack \
  -H "Content-Type: application/json" \
  -d '{}'
```

The message status transitions from `processing` to `deleted`.

## Next Steps

- Read the [Concepts](./concepts) guide to understand queue mechanics, visibility timeouts, and consumer groups
- Explore the [Authentication](./authentication) guide to enable JWT-based auth
- Check the [Client Libraries](../clients/go) to build applications that integrate with queue-ti
- Review [Deployment](./deployment) for production-ready configurations
