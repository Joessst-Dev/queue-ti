---
name: Go test DB connectivity pattern
description: How each test package connects to PostgreSQL — critical for CI design decisions (testcontainers vs service container)
type: project
---

The Go test suite uses two distinct PostgreSQL connection strategies across packages:

- `internal/db` and `internal/server` — use **testcontainers-go** to spin up their own `postgres:16-alpine` container programmatically in `BeforeSuite`. No external DB needed; Docker socket on `ubuntu-latest` is sufficient.
- `internal/queue` — hardcodes `postgres://postgres:postgres@localhost:5432/queueti_test` directly in `BeforeEach`. Requires a real service reachable at `localhost:5432`.
- `internal/config` and `internal/auth` — no database dependency.

**Why:** This split means CI must provide *both* Docker access (for testcontainers) *and* a GitHub Actions service container bound to port 5432, otherwise `internal/queue` tests fail with connection refused.

**How to apply:** When modifying the CI pipeline, always include both: `services.postgres` at port 5432 AND ensure the Docker daemon socket is available (it is by default on `ubuntu-latest`). Do not attempt to remove the service container thinking testcontainers covers everything — it does not.
