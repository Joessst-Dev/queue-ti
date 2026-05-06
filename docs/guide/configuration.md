# Configuration

Configuration is loaded from `config.yaml` at the repository root. All keys can be overridden with environment variables prefixed `QUEUETI_`.

## Configuration File

Create or edit `config.yaml`:

```yaml
server:
  port: 50051          # gRPC server port
  http_port: 8080      # HTTP admin API port

db:
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  name: queueti
  sslmode: disable     # Options: disable, require, verify-ca, verify-full

queue:
  visibility_timeout: 30s       # Time a dequeued message remains invisible to other consumers
  max_retries: 3                # Maximum number of retries for a failed message
  message_ttl: 24h              # Time-to-live for messages (0 = no expiry)
  dlq_threshold: 3              # Retry count at which messages are promoted to DLQ (0 = disabled); overridden per-topic by max_retries in topic config
  require_topic_registration: false  # Require explicit topic registration before enqueue (default: false)
  delete_reaper_schedule: ""    # Cron schedule for automatic expired message deletion (empty = disabled)

auth:
  enabled: false
  username: admin
  password: secret

log_level: info         # Log level: debug, info, warn, error (default: info)

# redis:
#   host: ""            # Redis host for login rate limiter (empty = in-memory, disabled by default)
#   port: 6379          # Redis port
#   password: ""        # Redis AUTH password (optional, but required in production)
#   tls_enabled: false  # Enable TLS for Redis connections
```

## Environment Variables

Any configuration key can be overridden with an environment variable using the key path with underscores and the `QUEUETI_` prefix:

| Variable | Description | Example |
|----------|-------------|---------|
| `QUEUETI_SERVER_PORT` | gRPC port | `50051` |
| `QUEUETI_SERVER_HTTP_PORT` | HTTP port | `8080` |
| `QUEUETI_DB_HOST` | PostgreSQL host | `localhost` |
| `QUEUETI_DB_PORT` | PostgreSQL port | `5432` |
| `QUEUETI_DB_USER` | PostgreSQL user | `postgres` |
| `QUEUETI_DB_PASSWORD` | PostgreSQL password | `postgres` |
| `QUEUETI_DB_NAME` | PostgreSQL database | `queueti` |
| `QUEUETI_DB_SSLMODE` | PostgreSQL SSL mode | `disable` |
| `QUEUETI_QUEUE_VISIBILITY_TIMEOUT` | Visibility timeout | `30s` |
| `QUEUETI_QUEUE_MAX_RETRIES` | Max retry count per message | `3` |
| `QUEUETI_QUEUE_MESSAGE_TTL` | Message time-to-live (0 = no expiry) | `24h` |
| `QUEUETI_QUEUE_DLQ_THRESHOLD` | Global retry count for DLQ promotion (0 = disabled); per-topic `max_retries` takes precedence when set | `3` |
| `QUEUETI_QUEUE_REQUIRE_TOPIC_REGISTRATION` | Require topics to be registered before enqueue | `false` |
| `QUEUETI_QUEUE_DELETE_REAPER_SCHEDULE` | Cron schedule for automatic expired message deletion (empty = disabled) | (empty) |
| `QUEUETI_AUTH_ENABLED` | Enable JWT authentication | `true` |
| `QUEUETI_AUTH_JWT_SECRET` | JWT signing secret (required if auth enabled) | (any string) |
| `QUEUETI_AUTH_USERNAME` | Default admin username | `admin` |
| `QUEUETI_AUTH_PASSWORD` | Default admin password | `secret` |
| `QUEUETI_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `QUEUETI_REDIS_HOST` | Redis host for login rate limiter (empty = in-memory; default: empty) | `` |
| `QUEUETI_REDIS_PORT` | Redis port | `6379` |
| `QUEUETI_REDIS_PASSWORD` | Redis AUTH password (optional, but recommended in production) | `` |
| `QUEUETI_REDIS_TLS_ENABLED` | Enable TLS for Redis connections | `false` |

## Log Levels

The `log_level` configuration controls the verbosity of server logging:

| Level | Use Case | Typical Output |
|-------|----------|-----------------|
| **debug** | Local development, detailed message tracing | Per-message operations (enqueue, dequeue, ack, nack-retry), HTTP requests |
| **info** | Production (default) | Server startup, DLQ promotions, requeue operations, expiry reaper results, auth enabled notice |
| **warn** | Production monitoring | Authentication failures, DLQ threshold misconfiguration |
| **error** | Production incidents | Unexpected DB failures, server errors |

Set via environment variable:

```bash
QUEUETI_LOG_LEVEL=debug
```

Or in `config.yaml`:

```yaml
log_level: debug
```

The resolved log level is printed at server startup.

## Topic Registration

By default, queue-ti allows messages to be enqueued to any topic without prior registration. This is convenient for development but can be risky in production—typos in topic names create silent, unrecoverable message loss.

To require explicit topic registration, enable the `require_topic_registration` flag:

```yaml
queue:
  require_topic_registration: true
```

Or via environment variable:

```bash
QUEUETI_QUEUE_REQUIRE_TOPIC_REGISTRATION=true
```

**Behavior when registration is required:**
- Enqueue requests to unregistered topics are rejected with HTTP 422 (gRPC `FailedPrecondition`)
- Topics are registered by creating a configuration entry via `PUT /api/topic-configs/:topic`
- The admin UI **New Topic** button (in the Topics section) simplifies registration; when enabled, admins must register a topic before producers can enqueue to it
- The empty-state message in the admin UI changes to: "No topics registered. Use 'New Topic' to register a topic before messages can be enqueued to it."

**Example: Register a topic and enqueue a message**

```bash
# Register the topic
curl -u admin:secret -X PUT http://localhost:8080/api/topic-configs/orders \
  -H "Content-Type: application/json" \
  -d '{}'

# Now enqueue is allowed
curl -X POST http://localhost:8080/api/messages \
  -H "Content-Type: application/json" \
  -d '{"topic": "orders", "payload": "eyJvcmRlcl9pZCI6IDEyMzQ1fQ=="}'
```

**When to enable registration:**
- Production deployments where topic names are fixed and controlled
- Microservices architectures with schema registries (topics are registered alongside schemas)
- Teams that want producer errors on typos rather than silent failures

## Redis Integration

Redis is optional but recommended for production deployments. When configured, Redis powers three critical features:

1. **Login rate limiter** — Prevents brute-force authentication attacks (shared state across instances)
2. **Distributed cache** — Two-tier caching for schema and topic config lookups (in-process + Redis)
3. **Cross-instance broadcaster** — Invalidates caches on all instances when schemas or configs change

### When Redis is Enabled vs. Disabled

**Without Redis:**
- Rate limiter: In-memory, per-instance (suitable for single-instance deployments)
- Cache: In-process only, invalidation via PostgreSQL LISTEN/NOTIFY
- Broadcaster: PostgreSQL LISTEN/NOTIFY (single-instance friendly)
- Trade-off: Simpler setup, but multiple instances don't share cached state — each instance has its own L1 cache

**With Redis:**
- Rate limiter: Shared Redis counter across all instances
- Cache: Two-tier (in-process L1 + Redis L2); L1 protects warm entries, Redis errors degrade gracefully to DB
- Broadcaster: Redis pub/sub (faster, more reliable for multi-instance)
- Trade-off: Additional infrastructure, but significant performance gains in multi-instance deployments

### Enabling Redis

To enable Redis, set the `redis.host` configuration:

```yaml
redis:
  host: redis.example.com  # Non-empty host enables Redis
  port: 6379
  password: ""             # Optional, but recommended for production
  tls_enabled: false       # Enable TLS for secure Redis connections
```

Or use environment variables:

```bash
QUEUETI_REDIS_HOST=redis.example.com
QUEUETI_REDIS_PORT=6379
QUEUETI_REDIS_PASSWORD=your-redis-password
QUEUETI_REDIS_TLS_ENABLED=true
```

When `QUEUETI_REDIS_HOST` is empty or unset, Redis is disabled, and the service falls back to in-memory storage and PostgreSQL LISTEN/NOTIFY.

### Distributed Caching

Schema and topic config lookups use a two-tier cache:

- **L1 (in-process)**: `sync.Map` of compiled schemas and configs, fastest access
- **L2 (Redis)**: JSON-serialized schemas and configs, shared across instances, survives restarts
- **L3 (PostgreSQL)**: Source of truth

**Cache keys:**
- Schemas: `queueti:cache:schema:<topic>` (TTL 30 seconds)
- Topic configs: `queueti:cache:topic_config:<topic>` (TTL 30 seconds)

When a schema or config does not exist in the database, a sentinel value (`"null"`) is stored in Redis to prevent repeated lookups for 30 seconds.

**Invalidation:**
- When a schema is registered, updated, or deleted, the local cache is evicted and a broadcast is sent to all instances
- When a topic config is created, updated, or deleted, the local cache is evicted and a broadcast is sent to all instances
- Without Redis, invalidation uses PostgreSQL LISTEN/NOTIFY (all instances receive the notification in real time)
- With Redis, invalidation uses Redis pub/sub (purpose-built for messaging, lower overhead than holding a dedicated DB connection per listener)

### Login Rate Limiter

The login endpoint (`POST /api/auth/login`) is protected by a rate limiter that prevents brute-force authentication attacks.

**Default behavior (in-memory):**
- Rate limit: 10 requests per 60-second window per client IP
- Storage: In-memory; each instance has its own rate-limit counter
- Suitable for: Single-instance deployments, development, testing

**With Redis (shared state):**
- Rate limit: 10 requests per 60-second window per client IP (same limit, shared across instances)
- Storage: Redis; all backend instances query the same counter
- Suitable for: Multi-replica deployments, load-balanced production setups

### Redis Connection Details

- **Startup validation**: The server pings Redis at startup to verify reachability. If the ping fails, the server logs an error and exits.
- **Client IP detection**: The rate limiter uses the `X-Real-IP` header (set by reverse proxies like Nginx) to identify the client. Ensure your proxy sets this header correctly.
- **Key isolation**: Rate-limit and cache keys are namespaced to prevent collisions.
- **Performance**: Redis is treated as an optimization layer. If Redis becomes unavailable after startup, the service continues to function with graceful degradation (in-process caching and PostgreSQL lookups).

### Example: Docker Compose with Redis

The `docker-compose.redis.yaml` overlay adds a Redis service and configures the backend to use it:

```bash
# Start with Redis
docker-compose -f docker-compose.yaml -f docker-compose.redis.yaml up -d

# Or use the convenient make target
make up-redis
```

This runs:
- PostgreSQL (as usual)
- Redis (7-alpine) bound to `127.0.0.1:6379`
- Backend (with `QUEUETI_REDIS_HOST=redis`, `QUEUETI_REDIS_PORT=6379`)
- Admin UI (as usual)

To stop all services:

```bash
make down
```

### Multi-Replica Deployments

In production with multiple backend replicas behind a load balancer:

1. **Configure Redis** to a shared instance (e.g., an AWS ElastiCache, Google Cloud Memorystore, or self-hosted Redis cluster)
2. **Set Redis credentials** via `QUEUETI_REDIS_PASSWORD` and optionally `QUEUETI_REDIS_TLS_ENABLED`
3. **Deploy replicas** — each instance connects to the same Redis for shared caching, broadcasting, and rate limiting
4. **Security note**: Bind your Redis instance to a private network or use authentication and TLS (`QUEUETI_REDIS_TLS_ENABLED=true`) in production

## Delete Reaper Schedule

The delete reaper runs automatically on a configurable cron schedule. Use standard 5-field cron syntax (minute, hour, day, month, day-of-week). The schedule can be configured in three ways:

1. **Static configuration** — Set at startup via `config.yaml` or `QUEUETI_QUEUE_DELETE_REAPER_SCHEDULE` env var
2. **Database storage** — The schedule is persisted in the `system_settings` table; this takes precedence over static config on subsequent restarts
3. **Runtime configuration** — Change the schedule live from the admin UI without restarting; the change applies immediately to the running instance

**Static configuration (config.yaml):**

```yaml
queue:
  delete_reaper_schedule: "0 2 * * *"  # 2:00 AM every day
```

Or via environment variable:

```bash
QUEUETI_QUEUE_DELETE_REAPER_SCHEDULE="0 2 * * *"
```

**Common schedules:**

| Schedule | When it runs |
|----------|--------------|
| `""` (empty) | Disabled (default) |
| `0 2 * * *` | Daily at 2:00 AM |
| `0 */6 * * *` | Every 6 hours |
| `0 0 1 * *` | First day of each month |
| `0 */2 * * *` | Every 2 hours |

**Runtime configuration (Admin UI):**

The admin UI's **Admin → Delete Reaper** section displays the current schedule and allows you to change it without restarting the server:
- Shows the active schedule and a status badge (Active / Not configured)
- Edit the cron expression in the input field
- Click **Save** to validate the cron syntax and apply the new schedule immediately
- A feedback message indicates success or displays the validation error

**How precedence works:**

On server startup:
1. If a schedule exists in the `system_settings` database table (from a prior Admin UI change), that schedule is used
2. Otherwise, the `QUEUETI_QUEUE_DELETE_REAPER_SCHEDULE` env var / `config.yaml` value is used
3. If both are absent or empty, the delete reaper is disabled

When the schedule is empty or disabled, the delete reaper only runs when triggered manually via `POST /api/admin/delete-reaper/run`.

**Multi-instance note:** When using multiple queue-ti instances behind a load balancer, a schedule change via the Admin UI updates the database and restarts the cron only on the instance that received the API request. Other instances will pick up the new schedule on their next restart. For immediate consistency across all instances, restart them after changing the schedule via the Admin UI.
