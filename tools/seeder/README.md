# queue-ti seeder

A one-shot binary that reads a JSON seed file and idempotently provisions queue-ti resources before your application starts. Run it as a Docker Compose init sidecar or from a CI step — it exits 0 on success and non-zero on any error.

## Usage

```sh
go run ./tools/seeder -f seed.json [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-f` / `--file` | (required) | Path to seed JSON file |
| `--admin-url` | `http://localhost:8080` | Base URL of the admin HTTP API |
| `--token` | `""` | Static bearer token |
| `--username` | `""` | Username for login-based auth |
| `--password` | `""` | Password — prefer `SEEDER_PASSWORD` env var to avoid ps exposure |
| `--dry-run` | `false` | Print planned actions without calling the API |
| `--timeout` | `30s` | Per-request HTTP timeout |

### Authentication

Auth credentials are resolved in this order:

1. `--token` — used as-is if set
2. `--username` + `--password` (or `SEEDER_PASSWORD` env var) — exchanges credentials for a JWT via `POST /api/auth/login`
3. No flags set — requests are sent unauthenticated (valid when auth is disabled on the server)

---

## Seed file format

The seed file is a JSON object with three optional top-level arrays. All sections are optional — omit any section you don't need.

```json
{
  "topic_configs": [...],
  "topic_schemas": [...],
  "consumer_groups": [...]
}
```

### `topic_configs`

Controls delivery behaviour for a topic. Each entry is upserted — applied every run, idempotent.

```json
"topic_configs": [
  {
    "topic": "orders",
    "max_retries": 3,
    "message_ttl_seconds": 3600,
    "replayable": true,
    "replay_window_seconds": 86400,
    "max_depth": 10000,
    "throughput_limit": 100
  }
]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `topic` | string | yes | Topic name. Must be non-empty. |
| `max_retries` | integer | no | Maximum delivery attempts before a message is moved to the dead-letter queue. Omit to use the server default. |
| `message_ttl_seconds` | integer | no | How long (in seconds) a message lives before being discarded. Omit for no TTL. |
| `replayable` | boolean | no | Whether messages on this topic can be replayed. Defaults to `false`. |
| `replay_window_seconds` | integer | no | How far back (in seconds) a replay can reach. Only meaningful when `replayable` is `true`. |
| `max_depth` | integer | no | Maximum number of messages allowed in the queue at one time. Omit for no limit. |
| `throughput_limit` | integer | no | Maximum messages processed per second on this topic. Omit for no limit. |

### `topic_schemas`

Registers a JSON Schema for a topic. Messages that do not conform to the schema are rejected at enqueue time. Each entry is upserted — applied every run, idempotent.

```json
"topic_schemas": [
  {
    "topic": "orders",
    "schema": "{\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"string\"},\"amount\":{\"type\":\"number\"}},\"required\":[\"id\"]}"
  }
]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `topic` | string | yes | Topic name. Must be non-empty. |
| `schema` | string | yes | A valid JSON Schema, serialised as a JSON string (i.e. the schema itself is escaped inside the outer JSON). |

> **Tip:** Generate the `schema` value with `jq -c . schema.json | jq -Rs .` to correctly escape the inner JSON.

### `consumer_groups`

Registers named consumer groups on a topic. Consumer groups isolate message delivery — each group receives its own independent copy of every message. Groups are created only when missing; existing groups are skipped.

```json
"consumer_groups": [
  {
    "topic": "orders",
    "groups": ["billing", "invoicing", "notifications"]
  }
]
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `topic` | string | yes | Topic name. Must be non-empty. |
| `groups` | array of strings | yes | Consumer group names to register. Empty arrays are silently skipped. |

---

## Conflict behaviour

| Resource | Strategy | Notes |
|----------|----------|-------|
| Topic configs | **Upsert** | Always applied. Safe to run repeatedly. |
| Topic schemas | **Upsert** | Always applied. Increments the schema version on the server. |
| Consumer groups | **Create-if-missing** | Existing groups are never modified or deleted. |

---

## Dry-run mode

Pass `--dry-run` to preview what would be applied without making any API calls:

```sh
go run ./tools/seeder -f seed.json --dry-run
```

Each planned action is logged to stderr. No requests are sent to the server.

---

## Docker Compose sidecar

```yaml
services:
  seeder:
    build:
      context: .
      dockerfile: tools/seeder/Dockerfile
    command: ["-f", "/seed.json", "--admin-url", "http://backend:8080", "--username", "admin"]
    environment:
      SEEDER_PASSWORD: "${ADMIN_PASSWORD}"
    volumes:
      - ./seed.json:/seed.json:ro
    depends_on:
      backend:
        condition: service_healthy

  backend:
    # ...
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/auth/status"]
      interval: 5s
      retries: 10
```

The seeder exits immediately after applying the seed file. Use `condition: service_completed_successfully` on any service that must wait for seeding to finish before it starts.

---

## Full example

See [`testdata/seed.json`](testdata/seed.json) for a complete seed file covering all three sections.
