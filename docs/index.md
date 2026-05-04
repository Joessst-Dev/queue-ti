---
layout: home

hero:
  name: queue-ti
  text: Self-hosted distributed message queue backed by PostgreSQL
  tagline: No Kafka, Redis, or RabbitMQ. If you have Postgres, you have a production-ready queue.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/Joessst-Dev/queue-ti

features:
  - icon: 🗄️
    title: PostgreSQL Only
    details: No additional infrastructure. If you run Postgres already, queue-ti is a drop-in message queue with one table.
  - icon: ✅
    title: At-Least-Once Delivery
    details: Messages are never lost. Visibility timeouts ensure unacked messages are retried. Dead-letter queue automatically contains exhausted messages.
  - icon: 📊
    title: Built for Observability
    details: Prometheus metrics out of the box; live queue depth via REST API; admin UI shows message status, retry counts, and expiry times.
  - icon: ⚡
    title: High Performance
    details: gRPC protocol with FOR UPDATE SKIP LOCKED dequeue. Throughput tested at 1500+ ops/sec per consumer.
  - icon: 🎯
    title: Admin UI Included
    details: Inspect messages, manually enqueue test data, requeue from DLQ, manage topics and users—all without writing code.
  - icon: 🔒
    title: JWT Authentication
    details: Optional JWT-based auth with user accounts, role-based access, and per-topic grants. OAuth-ready.
  - icon: 📦
    title: Multi-Language Clients
    details: Official clients for Go, Node.js, Python, and Java — auto-reconnect, token refresh, and batch consumption included.

---

## Quick Start

```bash
docker-compose up
```

Access the admin UI at `http://localhost:8081` (login: `admin` / `secret`).

## Why queue-ti?

queue-ti is designed for teams who want reliable, observable message processing without the operational overhead of a separate queue broker. It features:

- **Built-in dead-letter queues** — Failed messages are automatically moved to a DLQ for manual inspection and requeue
- **Per-topic schema validation** — Avro schemas enforce payload contracts at enqueue time
- **Fine-grained JWT grants** — Control access per topic, topic pattern, or consumer group
- **Throughput throttling** — Per-topic rate limits using PostgreSQL token-bucket algorithm
- **Consumer groups** — Multiple independent systems process the same messages in parallel
- **Message keys & upsert** — Deduplication and idempotent enqueue operations
- **Automatic reapers** — Expiry reaper marks old messages; delete reaper cleans up disk space
- **Multi-language clients** — Go, Node.js, and Python with auto-reconnect and token refresh

## Core Features

- **gRPC API** — High-performance queue operations (enqueue, dequeue, acknowledge, nack) over gRPC
- **HTTP Admin API** — REST endpoints for queue inspection, management, user/grant administration, and schema configuration
- **Topic-based routing** — Multiple independent queues share a single PostgreSQL table, partitioned by topic
- **Message keys** — Optional deduplication keys allow upsert semantics
- **Automatic retries** — Failed messages are automatically retried up to a configurable limit
- **Dead-letter queue** — Messages that exhaust their retry limit are automatically promoted to `<topic>.dlq`
- **Message TTL** — Messages expire after a configurable duration
- **Contention-free dequeue** — Uses `FOR UPDATE SKIP LOCKED` for lock-free concurrent consumption
- **JWT authentication** — Optional JWT-based auth with user accounts, role-based access, and per-topic grants
- **Avro schema validation** — Optional per-topic Avro schema registration; payloads validated at enqueue time
- **Per-topic configuration** — Override retry count, TTL, queue depth limits, and throughput caps per topic via HTTP API or admin UI
- **Throughput throttling** — Optional per-topic message rate limits (messages/second) enforced at dequeue time
- **Admin UI** — Angular web interface for message inspection, manual enqueue, DLQ requeue, and topic management
- **Prometheus metrics** — Real-time counters and gauges (`/metrics` endpoint, unauthenticated)
- **Client libraries** — Go, Node.js, and Python with async-first design, auto-reconnection, and token refresh
