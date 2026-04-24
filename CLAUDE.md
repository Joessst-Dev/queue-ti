# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**queue-ti** is a distributed message queue service with two components:
- **Backend** — Go gRPC service + HTTP admin API backed by PostgreSQL
- **Admin UI** — Angular SPA for queue management and message inspection

## Commands

### Backend (Go)

```bash
make proto        # Regenerate Go bindings from proto/queue.proto
make deps         # go mod tidy
make test         # Run all tests via Ginkgo (ginkgo ./...)
make run          # Start server (gRPC :50051, HTTP :8080)
```

Run a single test package:
```bash
ginkgo ./internal/queue/...
```

### Admin UI (Nx + Angular)

```bash
npx nx serve      # Dev server with hot reload
npx nx build      # Production build
npx nx test       # Unit tests (Vitest)
npx nx lint       # ESLint
```

### Full stack

```bash
docker-compose up   # PostgreSQL + backend + frontend (Nginx)
```

## Architecture

### Backend layers

```
cmd/server/main.go
├── gRPC server (port 50051)  — queue clients
└── HTTP server  (port 8080)  — admin UI + REST
    └── internal/
        ├── config/     Viper YAML + env vars (prefix QUEUETI_)
        ├── db/         pgx/v5 pool, golang-migrate migrations
        ├── queue/      Core logic: Enqueue / Dequeue / Ack / List
        ├── server/     gRPC + HTTP handlers wiring queue service
        ├── auth/       Basic-auth interceptor for gRPC
        └── pb/         Generated protobuf Go bindings
```

### Queue mechanics

- Single `messages` table (PostgreSQL); composite index on `(topic, status, visibility_timeout, created_at)` for efficient dequeue.
- Dequeue uses `FOR UPDATE SKIP LOCKED` for contention-free concurrent consumers.
- At-least-once delivery: unacked messages re-appear after the visibility timeout (default 30 s).
- Topic-based routing: independent queues share one table, partitioned by `topic`.

### Frontend layers

```
admin-ui/src/app/
├── services/
│   ├── queue.service.ts      HTTP client to backend :8080
│   └── auth.service.ts       Auth state
├── interceptors/
│   └── auth.interceptor.ts   Injects auth header on every request
├── guards/
│   └── auth.guard.ts         Protects routes
├── messages/                 Message list + inspection view
└── login/                    Authentication UI
```

The admin UI talks exclusively to the HTTP API (port 8080); gRPC (port 50051) is for queue producer/consumer clients only.

### Configuration

Local development: `config.yaml` at repo root. Any key can be overridden with an environment variable prefixed `QUEUETI_` (e.g. `QUEUETI_DB_DSN`). Docker Compose sets these for containerized deployment.

### Protobuf

The service contract lives in `proto/queue.proto`. After modifying it, run `make proto` to regenerate `internal/pb/`. Never hand-edit generated files in `pb/`.
