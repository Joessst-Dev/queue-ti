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
- **Go, Node.js, Python, Java, and C# clients** — auto-reconnect, token refresh, batch consumption

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
```

All clients include a `QueueTiAuth` helper (or `NewAuth` in Go) that automatically detects whether authentication is enabled, handles login, and manages token refresh—no boilerplate needed. See each client's README for usage.

**Java** (GitHub Packages — [see setup instructions](https://joessst-dev.github.io/queue-ti/clients/java)):

```kotlin
implementation("de.joesst.dev:queue-ti-java-client:VERSION")
```

Latest version: [github.com/Joessst-Dev/queue-ti-java-client/releases](https://github.com/Joessst-Dev/queue-ti-java-client/releases)

**C#** ([NuGet](https://www.nuget.org/packages/QueueTi.Client)):

```bash
dotnet add package QueueTi.Client
```

Latest version: [nuget.org/packages/QueueTi.Client](https://www.nuget.org/packages/QueueTi.Client)

### Example Applications

Each client library includes a **producer → consumer → ack** order pipeline example:

```bash
# Go
cd clients/go-client/examples/order-pipeline && go run main.go

# Node.js
cd clients/node && npx ts-node --transpile-only examples/order-pipeline/index.ts

# Python
cd clients/python && python -m examples.order_pipeline
```

The examples use built-in `QueueTiAuth` helpers and run against a local queue-ti instance (default credentials: `admin` / `secret`). Override with `QUEUETI_USERNAME` / `QUEUETI_PASSWORD` environment variables.

## License

MIT
