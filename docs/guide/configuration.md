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
  dlq_threshold: 3              # Retry count at which messages are promoted to DLQ (0 = disabled)
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
| `QUEUETI_QUEUE_DLQ_THRESHOLD` | Retry count for DLQ promotion (0 = disabled) | `3` |
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

## Redis Rate Limiter

The login endpoint (`POST /api/auth/login`) is protected by a rate limiter that prevents brute-force authentication attacks. By default, the rate limiter uses in-memory storage. For multi-replica deployments, configure Redis to share rate-limit state across all backend instances.

**Default behavior (in-memory):**
- Rate limit: 10 requests per 60-second window per client IP
- Storage: In-memory; each instance has its own rate-limit counter
- Suitable for: Single-instance deployments, development, testing

**With Redis (shared state):**
- Rate limit: 10 requests per 60-second window per client IP (same limit, shared across instances)
- Storage: Redis; all backend instances query the same counter
- Suitable for: Multi-replica deployments, load-balanced production setups

### Enabling Redis Rate Limiter

To enable Redis-backed rate limiting, set the `redis.host` configuration:

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

When `QUEUETI_REDIS_HOST` is empty or unset, the rate limiter falls back to in-memory storage automatically.

### Redis Connection

- **Startup validation**: The server pings Redis at startup to verify reachability. If the ping fails, the server logs an error and exits.
- **Client IP detection**: The rate limiter uses the `X-Real-IP` header (set by reverse proxies like Nginx) to identify the client. Ensure your proxy sets this header correctly.
- **Key isolation**: Rate-limit keys include the client IP to prevent one user's failed login attempts from blocking others.

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
3. **Deploy replicas** — each instance connects to the same Redis and shares rate-limit state
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
