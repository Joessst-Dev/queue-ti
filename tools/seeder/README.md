# queue-ti seeder

A one-shot binary that reads a JSON seed file and idempotently provisions topic configurations, topic schemas, and consumer groups against a running queue-ti instance via the admin HTTP API. Run it as an init sidecar before your application starts.

## Usage

```sh
go run ./tools/seeder -f seed.json [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-f` / `-file` | (required) | Path to seed JSON file |
| `-admin-url` | `http://localhost:8080` | Base URL of the admin HTTP API |
| `-token` | `""` | Static bearer token |
| `-username` | `""` | Username for login-based auth |
| `-password` | `""` | Password (prefer `SEEDER_PASSWORD` env var to avoid ps exposure) |
| `-dry-run` | `false` | Print planned actions without calling the API |
| `-timeout` | `30s` | Per-request HTTP timeout |

## Seed file format

```json
{
  "topic_configs": [
    {
      "topic": "orders",
      "max_retries": 3,
      "message_ttl_seconds": 3600,
      "replayable": true,
      "max_depth": 10000,
      "replay_window_seconds": 86400,
      "throughput_limit": 100
    }
  ],
  "topic_schemas": [
    {
      "topic": "orders",
      "schema": "{\"type\":\"object\",\"properties\":{\"id\":{\"type\":\"string\"}}}"
    }
  ],
  "consumer_groups": [
    {
      "topic": "orders",
      "groups": ["billing", "invoicing"]
    }
  ]
}
```

All top-level sections are optional. See `testdata/seed.json` for a full example.

## Conflict behaviour

| Resource | Strategy |
|----------|----------|
| Topic configs | Upsert — always applied, idempotent |
| Topic schemas | Upsert — always applied, idempotent |
| Consumer groups | Create-if-missing — existing groups are skipped |

## Docker Compose sidecar

```yaml
services:
  seeder:
    build:
      context: .
      dockerfile: tools/seeder/Dockerfile
    command: ["-f", "/seed.json", "-admin-url", "http://backend:8080"]
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
