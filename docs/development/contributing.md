# Contributing

To contribute to queue-ti, follow these steps:

## Prerequisites

- Go 1.25.5 or later
- PostgreSQL 16+
- Node.js 20+ (for admin UI development)
- Docker and Docker Compose (optional, recommended for easy setup)

## Local Setup

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
```

Or use Docker Compose:

```bash
docker-compose up postgres
```

### 3. Start the backend

```bash
make run
```

The server listens on:
- **gRPC**: `localhost:50051`
- **HTTP**: `localhost:8080`

### 4. Start the admin UI (in another terminal)

```bash
cd admin-ui
npm install
npx nx serve
```

The UI is available at `http://localhost:4200`

## Development Workflow

### Backend (Go)

**Running tests:**

```bash
# Run all tests
make test

# Run tests for a specific package
ginkgo ./internal/queue/...

# Run tests with coverage
ginkgo ./... -cover
```

Tests use TestContainers to spin up a real PostgreSQL instance; no mocking of the database.

**Regenerating protobuf:**

After modifying `proto/queue.proto`:

```bash
make proto
```

This regenerates `internal/pb/queue.pb.go` and `internal/pb/queue_grpc.pb.go`. **Never hand-edit files in `pb/`.**

**Dependency management:**

```bash
# Update Go dependencies
make deps

# View dependency tree
go mod graph
```

**Code style:**

- Follow Go conventions (gofmt, golint)
- Write idiomatic Go with clear error handling
- Add meaningful comments for exported functions and types

### Frontend (Angular, Nx)

**Running tests:**

```bash
cd admin-ui
npx nx test
```

**Linting:**

```bash
npx nx lint
```

**Building for production:**

```bash
npx nx build
```

**Dependency updates:**

```bash
cd admin-ui
npm update
```

## Git Workflow

1. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```

2. **Make your changes** and commit regularly with clear messages:
   ```bash
   git commit -m "feat: add feature X"
   git commit -m "fix: resolve bug Y"
   git commit -m "refactor: improve Z"
   ```

3. **Run tests before pushing:**
   ```bash
   make test              # Backend
   cd admin-ui && npx nx test  # Frontend
   ```

4. **Push to your fork and open a pull request:**
   ```bash
   git push origin feat/your-feature-name
   ```

5. **Ensure CI passes** — All checks (tests, linting, builds) must pass before merging.

## Commit Message Format

Use descriptive commit messages following conventional commits:

- `feat:` — A new feature
- `fix:` — A bug fix
- `refactor:` — Code refactoring without changing behavior
- `test:` — Test additions or improvements
- `docs:` — Documentation updates
- `chore:` — Dependency updates, CI configuration, etc.

Examples:

```
feat: add consumer group support for per-team isolation
fix: resolve race condition in dequeue for high concurrency
refactor: simplify message lifecycle state machine
test: add integration tests for DLQ behavior
docs: clarify visibility timeout semantics
```

## Pull Request Guidelines

1. **Title**: Use the same format as commit messages (e.g., "feat: add X")
2. **Description**: Explain the "why" — what problem does this solve? What trade-offs were made?
3. **Testing**: Include test coverage for new features or bug fixes
4. **Documentation**: Update README or docs if behavior changes
5. **Review**: Ensure at least one approval before merging

## Performance and Benchmarks

### Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchtime=3s -run=^$ ./internal/queue/...

# Run a specific benchmark
go test -bench=BenchmarkEnqueue -benchtime=5s -run=^$ ./internal/queue/...

# Include memory allocation stats
go test -bench=. -benchmem -run=^$ ./internal/queue/...
```

Available benchmarks:

| Benchmark | What it measures |
|---|---|
| `BenchmarkEnqueue` | Sequential single-goroutine enqueue throughput |
| `BenchmarkEnqueueParallel` | Concurrent enqueue across `GOMAXPROCS` goroutines |
| `BenchmarkDequeueAck` | Dequeue + Ack hot path (pre-seeded queue, no enqueue overhead) |
| `BenchmarkFullPipeline` | Full Enqueue → Dequeue → Ack round-trip under parallel load |

### End-to-End Load Test

```bash
# Start the full stack
docker-compose up

# Run the load test
go run ./cmd/loadtest --producers=4 --consumers=4 --duration=30s

# With authentication
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' | jq -r '.token')

go run ./cmd/loadtest --token=$TOKEN --producers=8 --consumers=8
```

See [Performance Testing](../guide/performance-testing) for detailed load test options and interpretation.

## Code Quality

- **Linting**: Follow Go conventions; enable `golangci-lint` in your editor
- **Testing**: Aim for >80% test coverage on critical paths
- **Documentation**: Write clear comments and update docs for user-facing changes
- **Security**: Use parameterized queries, validate inputs, avoid hardcoded secrets

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
