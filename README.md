# queue-ti

A self-hosted distributed message queue backed by PostgreSQL — no Kafka, Redis, or RabbitMQ required.

If you already run Postgres, you have a production-ready queue. One table, at-least-once delivery, dead-letter queues, per-topic schema validation, and a browser-based admin UI out of the box.

→ **[Full documentation](https://joessst-dev.github.io/queue-ti)**

## Why queue-ti?

- **Zero extra infrastructure** — your existing Postgres instance is all you need
- **At-least-once delivery** — visibility timeouts and automatic retries; nothing gets lost
- **Dead-letter queue** — exhausted messages land in `<topic>.dlq` for inspection and requeue
- **Consumer groups** — multiple independent systems process the same topic in parallel
- **Fine-grained access control** — JWT auth with per-topic grants and consumer group restrictions
- **Avro schema validation** — enforce payload contracts at enqueue time
- **Admin UI** — inspect messages, requeue from DLQ, manage topics and users without writing code
- **Go, Node.js, Python, and Java clients** — auto-reconnect, token refresh, batch consumption

## Quick Start

```bash
docker-compose up
```

Access the admin UI at `http://localhost:8081` (login: `admin` / `secret`).

## Client Libraries

```bash
go get github.com/Joessst-Dev/queue-ti/clients/go-client
npm install @queue-ti/client
pip install queue-ti-client
# Java: see docs for GitHub Packages setup
implementation("de.joesst.dev:queue-ti-java-client:VERSION")
```

## License

MIT
