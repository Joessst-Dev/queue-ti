# Seeder

The seeder is a one-shot binary that reads a JSON seed file and idempotently provisions queue-ti resources before your application starts. Use it to declare the topics, schemas, and consumer groups your application needs without writing any setup code.

Typical usage is as a Docker Compose init sidecar that runs once, exits 0, and lets your application containers start with a fully configured queue.

## Installation

The seeder lives in the queue-ti repository and is built from source:

```sh
# Run directly
go run ./tools/seeder -f seed.json

# Build a binary
go build -o seeder ./tools/seeder
./seeder -f seed.json
```

A `Dockerfile` is included for containerised use — see [Docker Compose sidecar](#docker-compose-sidecar) below.

## Flags

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

## Seed file format

The seed file is a JSON object with three optional top-level arrays. Omit any section you don't need.

```json
{
  "topic_configs": [...],
  "topic_schemas": [...],
  "consumer_groups": [...]
}
```

### `topic_configs`

Controls delivery behaviour for a topic. Each entry is **upserted** — applied on every run, safe to repeat.

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

Registers a JSON Schema for a topic. Messages that do not conform are rejected at enqueue time. Each entry is **upserted** — applied on every run, safe to repeat.

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
| `schema` | string | yes | A valid JSON Schema, serialised as a JSON string (the schema itself is escaped inside the outer JSON). |

::: tip Escaping the schema
Generate the `schema` value from a file with:
```sh
jq -c . schema.json | jq -Rs .
```
:::

For more detail on schema validation behaviour see [Schema Validation](./schema-validation).

### `consumer_groups`

Registers named consumer groups on a topic. Each group receives an independent copy of every message — groups are the primary way to fan out work across multiple services. Groups are **created only when missing**; existing groups are never modified or deleted.

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

For background on consumer group semantics see [Consumer Groups](./consumer-groups).

## Conflict behaviour

| Resource | Strategy | Notes |
|----------|----------|-------|
| Topic configs | **Upsert** | Always applied. Safe to run repeatedly. |
| Topic schemas | **Upsert** | Always applied. Increments the schema version on the server. |
| Consumer groups | **Create-if-missing** | Existing groups are never modified or deleted. |

## Dry-run mode

Pass `--dry-run` to preview what would be applied without making any API calls:

```sh
go run ./tools/seeder -f seed.json --dry-run
```

Each planned action is logged to stderr. No requests are sent to the server. Useful for validating a seed file in CI before deploying.

## Docker Compose sidecar

Build the seeder image from the repository root (the `Dockerfile` requires the workspace context):

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

  your-app:
    image: your-app:latest
    depends_on:
      seeder:
        condition: service_completed_successfully

  backend:
    image: ghcr.io/joessst-dev/queue-ti:latest
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/auth/status"]
      interval: 5s
      retries: 10
```

`condition: service_completed_successfully` ensures `your-app` only starts once seeding has finished without errors.

## Kubernetes init container

The same pattern works as a Kubernetes init container:

```yaml
initContainers:
- name: seeder
  image: ghcr.io/joessst-dev/queue-ti-seeder:latest
  args: ["-f", "/config/seed.json", "--admin-url", "http://queue-ti:8080", "--username", "admin"]
  env:
  - name: SEEDER_PASSWORD
    valueFrom:
      secretKeyRef:
        name: queue-ti-auth
        key: password
  volumeMounts:
  - name: seed-config
    mountPath: /config
volumes:
- name: seed-config
  configMap:
    name: queue-ti-seed
```
