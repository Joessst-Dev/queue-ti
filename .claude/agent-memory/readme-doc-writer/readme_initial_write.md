---
name: README Initial Documentation
description: First comprehensive README for queue-ti project covering all major components
type: project
---

## Initial README Documentation (2026-04-25)

Created a comprehensive 556-line README at `/Users/jost.weyers/Documents/dev/queue-ti/README.md` covering:

### Sections Documented

1. **Project Overview** — Distributed message queue with Go gRPC backend + Angular admin UI
2. **Features** — All implemented features verified against source:
   - gRPC API (Enqueue, Dequeue, Ack)
   - HTTP Admin API (health, auth status, list messages, enqueue)
   - Topic-based routing
   - At-least-once delivery with 30s default visibility timeout
   - Contention-free dequeue via FOR UPDATE SKIP LOCKED
   - Basic auth for both gRPC and HTTP (disabled by default)
   - Admin UI (Angular in Nx workspace)
   - YAML config + QUEUETI_ env var prefix (Viper)

3. **Quick Start** — Copy-paste ready instructions:
   - Local dev (manual PostgreSQL + make run + npx nx serve)
   - Docker Compose single command setup
   - Admin UI accessible at localhost:4200 (dev) or localhost:8081 (Compose)

4. **Configuration Reference** — All config keys from `internal/config/config.go`:
   - server.port (50051), server.http_port (8080)
   - db.* (host, port, user, password, name, sslmode)
   - queue.visibility_timeout (default 30s)
   - auth.* (enabled, username, password)
   - Table of QUEUETI_ environment variables

5. **Architecture** — Reflects actual code structure:
   - Backend: Two concurrent servers (gRPC:50051, HTTP:8080) sharing one queue.Service
   - Queue mechanics: Single messages table with composite index (topic, status, visibility_timeout, created_at)
   - Dequeue algorithm using FOR UPDATE SKIP LOCKED
   - Message lifecycle (pending → processing → deleted / requeue on timeout)
   - Frontend: Angular SPA communicates exclusively to HTTP API

6. **API Reference** — Derived from proto/queue.proto and server implementations:
   - gRPC service (Enqueue, Dequeue, Ack with request/response messages)
   - HTTP endpoints: /healthz, /api/auth/status, GET /api/messages (with topic filter), POST /api/messages
   - Basic auth explained for both gRPC and HTTP

7. **Running Tests** — Commands verified against Makefile and package.json:
   - Backend: make test (Ginkgo), ginkgo ./path (specific package), uses TestContainers for real PostgreSQL
   - Frontend: npx nx test (Vitest)

8. **Development Workflow** — Key tasks:
   - make proto (regenerate pb/ from proto/queue.proto, warns never to hand-edit pb/)
   - make deps (go mod tidy)
   - Frontend deps: npm update in admin-ui/

9. **Project Structure** — Full tree with annotations for each directory

10. **Deployment** — Docker and Docker Compose instructions with example env vars

11. **Known Limitations** — Honest assessment:
    - No priority queues (FIFO only)
    - No message expiration (indefinite storage)
    - Single table design (consider partitioning for scale)
    - No dead-letter queue
    - No message scheduling

12. **Troubleshooting** — Common issues and diagnostics

### Key Technical Details Verified

- Go version: 1.25.5 (from go.mod)
- Angular version: ~21.2.0, Nx: 22.6.5, TypeScript: ~5.9.2
- Database: PostgreSQL 16-alpine (docker-compose.yaml)
- Docker Compose exposes:
  - 5432 (PostgreSQL)
  - 50051 (gRPC)
  - 8080 (HTTP)
  - 8081 (Admin UI via Nginx)
- Default Docker Compose auth: admin/secret (enabled in docker-compose.yaml, disabled in config.yaml)
- Fiber v2 (gofiber/fiber/v2) used for HTTP server, not standard library
- Protobuf import: google/protobuf/timestamp.proto (timestamppb.New in gRPC response)
- Basic auth is optional; both gRPC interceptor and HTTP middleware support it

### Architectural Decisions Documented

- Two-server model (gRPC + HTTP) is intentional: gRPC for performance, HTTP for admin UI
- All configuration via Viper with environment variable support
- At-least-once delivery (no exactly-once)
- Topic-partitioned (same table, filtered by topic) rather than separate tables
- Index on (topic, status, visibility_timeout, created_at) for dequeue efficiency
- Message payload stored as BYTEA; metadata as JSONB; both serialized in application layer

### Future Documentation Needs

- ADR for queue mechanics decision (FOR UPDATE SKIP LOCKED vs alternatives)
- ADR for two-server (gRPC + HTTP) design
- Migration guide if visibility_timeout becomes configurable per-topic
- Scaling guide for when single messages table becomes a bottleneck
