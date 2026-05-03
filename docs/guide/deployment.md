# Deployment

## Docker

### Pull the Latest Released Image

The easiest way to run queue-ti is to pull a pre-built Docker image from GitHub Container Registry (GHCR). Releases are published automatically and include backend, admin UI, and all dependencies.

```bash
# Latest stable release
docker pull ghcr.io/joessst-dev/queue-ti:latest

# Or a specific version (e.g. v2026.05.0-preview.1)
docker pull ghcr.io/joessst-dev/queue-ti:v2026.05.0-preview.1
```

### Run with Docker

```bash
docker run -d \
  -p 50051:50051 \
  -p 8080:8080 \
  -e QUEUETI_DB_HOST=postgres \
  -e QUEUETI_DB_USER=postgres \
  -e QUEUETI_DB_PASSWORD=postgres \
  -e QUEUETI_DB_NAME=queueti \
  ghcr.io/joessst-dev/queue-ti:latest
```

### Build Locally from Source

```bash
docker build -t queue-ti:dev .
docker run -d \
  -p 50051:50051 \
  -p 8080:8080 \
  -e QUEUETI_DB_HOST=postgres \
  -e QUEUETI_DB_USER=postgres \
  -e QUEUETI_DB_PASSWORD=postgres \
  -e QUEUETI_DB_NAME=queueti \
  queue-ti:dev
```

## gRPC TLS

The gRPC server (port 50051) runs **without TLS by default**. In production, never expose port 50051 directly to untrusted networks. Use one of the following approaches:

- **TLS-terminating reverse proxy** — Place an Envoy sidecar, an nginx stream proxy, or a cloud load balancer in front of port 50051 and have it handle TLS termination before forwarding plaintext gRPC to the backend.
- **Native TLS (planned)** — A future release will support loading a certificate and key directly in the server via `QUEUETI_GRPC_TLS_CERT` / `QUEUETI_GRPC_TLS_KEY` env vars. Until then, the reverse-proxy approach is the recommended workaround for production deployments.

The `docker-compose.yaml` already restricts gRPC to `127.0.0.1:50051` to prevent accidental external exposure in local and single-host environments.

## Docker Compose

The included `docker-compose.yaml` orchestrates PostgreSQL, the backend, and the admin UI. An optional Compose overlay, `docker-compose.redis.yaml`, adds a Redis service for shared login rate limiting.

### Without Redis (in-memory rate limiter)

```bash
make up
# or
docker-compose up -d
```

### With Redis (shared rate limiter — recommended for multi-replica deployments)

```bash
make up-redis
# or
docker-compose -f docker-compose.yaml -f docker-compose.redis.yaml up -d
```

The `docker-compose.redis.yaml` overlay adds a `redis:7-alpine` service (bound to `127.0.0.1:6379`) and wires `QUEUETI_REDIS_HOST` and `QUEUETI_REDIS_PORT` environment variables into the backend. When the overlay is active, all backend instances (if replicated) share the same login rate-limit state.

### Stop All Services

Works with or without the Redis overlay:

```bash
make down
```

### Additional make targets

- `make build-nocache` — Rebuild Docker images without cache (without Redis)
- `make build-nocache-redis` — Rebuild Docker images without cache (with Redis overlay)

Access the admin UI at `http://localhost:8081` (login: `admin` / `secret`).

## Multi-Instance Deployments

For production deployments with multiple queue-ti instances behind a load balancer:

1. **Database** — Use a managed PostgreSQL service (AWS RDS, Google Cloud SQL, etc.) or a highly available PostgreSQL cluster
2. **Load Balancer** — Place instances behind a load balancer for HTTP traffic (port 8080)
3. **gRPC Routing** — For gRPC clients, use a gRPC-aware load balancer (e.g., Envoy, AWS NLB with gRPC support) or direct DNS to backend instances
4. **Redis** — Configure a shared Redis instance for login rate limiting across all instances
5. **Schema/Config Invalidation** — Changes to topic schemas or configurations are broadcast via PostgreSQL LISTEN/NOTIFY to all instances in real-time

### Example Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: queue-ti
spec:
  replicas: 3
  selector:
    matchLabels:
      app: queue-ti
  template:
    metadata:
      labels:
        app: queue-ti
    spec:
      containers:
      - name: queue-ti
        image: ghcr.io/joessst-dev/queue-ti:latest
        ports:
        - containerPort: 50051
          name: grpc
        - containerPort: 8080
          name: http
        env:
        - name: QUEUETI_DB_HOST
          value: postgres.default.svc.cluster.local
        - name: QUEUETI_DB_USER
          valueFrom:
            secretKeyRef:
              name: queue-ti-db
              key: username
        - name: QUEUETI_DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: queue-ti-db
              key: password
        - name: QUEUETI_DB_NAME
          value: queueti
        - name: QUEUETI_REDIS_HOST
          value: redis.default.svc.cluster.local
        - name: QUEUETI_REDIS_PORT
          value: "6379"
        - name: QUEUETI_AUTH_ENABLED
          value: "true"
        - name: QUEUETI_AUTH_JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: queue-ti-auth
              key: jwt_secret
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
```

## Security Best Practices

1. **Enable authentication** — Use JWT with strong secrets in production
2. **Use TLS** — Terminate TLS at the load balancer or reverse proxy
3. **Network isolation** — Restrict gRPC (port 50051) to internal networks only
4. **Database credentials** — Store in Kubernetes secrets, environment variables, or a secrets manager; never commit to version control
5. **Redis credentials** — If using Redis for rate limiting, enable authentication and TLS
6. **Metrics endpoint** — Protect `/metrics` at the network level or with a reverse proxy
7. **Regular backups** — Backup your PostgreSQL database regularly
