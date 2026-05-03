# Authentication & User Management

queue-ti supports JWT-based authentication with per-user grants to enforce granular access control. User accounts and role assignments are managed via the admin UI or REST API.

## Enabling Authentication

Authentication is disabled by default. To enable it, set:

```yaml
auth:
  enabled: true
  jwt_secret: "your-secret-key-here"
```

Or use environment variables:

```bash
QUEUETI_AUTH_ENABLED=true
QUEUETI_AUTH_JWT_SECRET="your-secret-key-here"
```

The server will fail to start if `auth.enabled=true` and `jwt_secret` is empty.

## Default Admin User

On first startup, the server automatically seeds a default admin user from the configuration:

```yaml
auth:
  enabled: true
  jwt_secret: "your-secret-key-here"
  username: admin          # Becomes the first admin account username
  password: secret         # Becomes the first admin account password
```

After the server starts, the default user is created (if it doesn't already exist) with `is_admin=true`. You can change the password and create additional users via the admin UI **Users** tab.

## User Roles and Permissions

### Admin Flag

The `is_admin` flag grants a user unrestricted access:
- **Admin users** (`is_admin=true`) bypass all per-topic grant checks and can access all queue operations and admin endpoints
- **Regular users** (`is_admin=false`) are subject to per-topic grants

### Per-Topic Grants

Regular users require explicit grants for each action and topic. A grant specifies:
- **Action**: one of `read`, `write`, or `admin`
- **Topic Pattern**: one of the following:
  - `*` — All topics (wildcard grant)
  - `orders.*` — Prefix glob (e.g., matches `orders`, `orders.dlq`, `orders.v1`, etc.)
  - `orders` — Exact topic name

#### Grant Semantics

- `read` — Allows `GET /api/messages` and `GET /api/stats` for this topic
- `write` — Allows `POST /api/messages`, `POST /api/messages/:id/ack`, `POST /api/messages/:id/nack`, `POST /api/messages/:id/requeue` for this topic; also required for gRPC `Dequeue` calls. Note: `write` alone does not restrict which consumer groups are accessible.
- `admin` — Allows topic configuration and schema management (`GET/PUT/DELETE /api/topic-configs/:topic`, `GET/PUT/DELETE /api/topic-schemas/:topic`) for this topic
- `consume` — Restricts the user to a specific named consumer group on the topic. See **Consumer Group Grants** below.

#### Example Grants

| Username | Grant | Topic Pattern | Consumer Group | Interpretation |
|----------|-------|---------------|----------------|-----------------|
| `alice` | `write` | `orders` | — | Can enqueue, dequeue, ack, and nack messages in `orders` (no group restriction) |
| `bob` | `read` | `*` | — | Can list and inspect all messages across all topics |
| `charlie` | `admin` | `payments.*` | — | Can manage configuration for topics matching `payments.*` |
| `diana` | `consume` | `orders.*` | `warehouse` | Can only dequeue/ack/nack in `orders.*` topics when using consumer group `warehouse` |

## Consumer Group Grants

Consumer group grants let you restrict a user to a specific named consumer group on a topic. This is useful when multiple teams consume from the same topic and you want to enforce that each team only processes its own group.

### Semantics

- A user with no `consume` grants for a topic can consume from any group (unrestricted).
- Once any `consume` grant exists for a user+topic, that user is restricted to only the explicitly granted groups.
- A user can hold multiple `consume` grants for the same topic with different groups to allow access to more than one.
- A `write` grant alone does not restrict groups. If a user has both `write` and `consume` grants for a topic, the `consume` grant restrictions apply.

### Creating a Consumer Group Grant via REST

```bash
curl -X POST -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"topic_pattern": "orders.*", "consumer_group": "warehouse"}' \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/consumer-group-grants
```

Omitting `topic_pattern` defaults to `"*"` (all topics).

**Response:** HTTP 201 Created

```json
{
  "id": "...",
  "user_id": "...",
  "action": "consume",
  "topic_pattern": "orders.*",
  "consumer_group": "warehouse",
  "created_at": "2025-01-15T12:00:00Z"
}
```

Consumer group grants can be deleted via the existing `DELETE /api/users/:id/grants/:grant_id` endpoint.

## JWT Token Details

- **Token lifetime**: 15 minutes
- **Algorithm**: HS256 (HMAC with SHA-256)
- **Claims**:
  - `uid` — User UUID
  - `sub` — Username
  - `adm` — Boolean indicating if user is admin
  - `iat` — Issued at (standard)
  - `exp` — Expiration time (standard)

## Authentication Endpoints

### POST /api/auth/login

Authenticates a user and returns a JWT token.

**Request:**

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "secret"}'
```

**Response:** HTTP 200 OK

```json
{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}
```

### POST /api/auth/refresh

Refreshes an existing JWT token.

**Request:**

```bash
curl -X POST http://localhost:8080/api/auth/refresh \
  -H "Authorization: Bearer <token>"
```

**Response:** HTTP 200 OK with a new token

```json
{"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}
```

### GET /api/auth/status

Returns the current authentication status and the authenticated user.

**Request:**

```bash
curl http://localhost:8080/api/auth/status
```

**Response:** HTTP 200 OK

```json
{
  "auth_required": true,
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "username": "admin",
    "is_admin": true
  }
}
```

## User Management Endpoints (Admin-Only)

All user and grant management endpoints require `is_admin=true`.

### GET /api/users

Lists all user accounts.

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/users
```

**Response:**

```json
{
  "users": [
    {"id": "550e8400...", "username": "admin", "is_admin": true, "created_at": "2025-04-25T12:00:00Z"},
    {"id": "660e8400...", "username": "alice", "is_admin": false, "created_at": "2025-04-25T12:05:00Z"}
  ]
}
```

### POST /api/users

Creates a new user account.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"username": "bob", "password": "secure-password", "is_admin": false}' \
  http://localhost:8080/api/users
```

**Response:** HTTP 201 Created

```json
{"id": "770e8400...", "username": "bob", "is_admin": false, "created_at": "2025-04-25T12:10:00Z"}
```

### PUT /api/users/:id

Updates a user account (username, password, and/or admin flag).

```bash
curl -X PUT -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"is_admin": true}' \
  http://localhost:8080/api/users/770e8400-e29b-41d4-a716-446655440002
```

**Response:** HTTP 200 OK with the updated user

### DELETE /api/users/:id

Deletes a user account.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/770e8400-e29b-41d4-a716-446655440002
```

**Response:** HTTP 204 No Content

## Grant Management Endpoints (Admin-Only)

### GET /api/users/:id/grants

Lists all grants for a specific user.

```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/grants
```

**Response:**

```json
{
  "grants": [
    {
      "id": "880e8400...",
      "user_id": "550e8400...",
      "action": "write",
      "topic_pattern": "orders",
      "created_at": "2025-04-25T12:00:00Z"
    },
    {
      "id": "991e8400...",
      "user_id": "550e8400...",
      "action": "consume",
      "topic_pattern": "orders.*",
      "consumer_group": "warehouse",
      "created_at": "2025-04-25T12:01:00Z"
    }
  ]
}
```

### POST /api/users/:id/grants

Creates a new grant for a user.

```bash
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"action": "write", "topic_pattern": "payments.*"}' \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/grants
```

**Response:** HTTP 201 Created

### DELETE /api/users/:id/grants/:grant_id

Deletes a specific grant.

```bash
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/grants/880e8400-e29b-41d4-a716-446655440003
```

**Response:** HTTP 204 No Content

### POST /api/users/:id/consumer-group-grants

Creates a `consume` grant that restricts the user to a specific consumer group on a topic. Requires admin.

```bash
curl -X POST -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"topic_pattern": "orders.*", "consumer_group": "warehouse"}' \
  http://localhost:8080/api/users/550e8400-e29b-41d4-a716-446655440000/consumer-group-grants
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `consumer_group` | Yes | — | The consumer group the user is restricted to |
| `topic_pattern` | No | `*` | Topic pattern (`*`, `prefix.*`, or exact name) |

**Response:** HTTP 201 Created — returns the created grant object with `action: "consume"`.

Returns 400 if `consumer_group` is missing, 409 if the same user+topic+group combination already exists.

## Using JWT Tokens in HTTP Requests

After logging in, include the JWT token in the `Authorization: Bearer` header:

```bash
# Login to get token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "secret"}' | jq -r '.token')

# Use token for authenticated requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/messages
```

## Using JWT Tokens in gRPC Requests

For gRPC clients, include the JWT token in the `authorization` metadata header:

```go
import "google.golang.org/grpc/metadata"

// After login via HTTP to get the token
md := metadata.Pairs("authorization", "Bearer "+token)
ctx := metadata.NewOutgoingContext(context.Background(), md)

// Use ctx in gRPC calls
response, err := client.Enqueue(ctx, &pb.EnqueueRequest{...})
```

## Admin UI Authentication

The admin UI stores JWT tokens in `sessionStorage` after successful login. Tokens are automatically included in all HTTP requests via the auth interceptor.

**Login flow:**

1. User navigates to the login page
2. Enters username and password
3. Calls `POST /api/auth/login` with credentials
4. Token is stored in `sessionStorage`
5. User is redirected to the messages dashboard
6. Auth interceptor automatically adds `Authorization: Bearer <token>` to all subsequent requests

**Token expiration:**

- When a token expires (15 minutes), the next API request returns HTTP 401
- The admin UI prompts the user to log in again
- Users can manually refresh tokens via `POST /api/auth/refresh`

## Security Notes

> **Warning**: Never commit the JWT secret to version control. Use a `.env` file or a secrets management system (e.g., Docker secrets, Kubernetes secrets) in production.

## Login Rate Limiting

The login endpoint (`POST /api/auth/login`) is protected by a rate limiter that prevents brute-force authentication attacks. By default, the rate limiter uses in-memory storage. For multi-replica deployments, configure Redis to share rate-limit state across all backend instances.

**Default behavior (in-memory):**
- Rate limit: 10 requests per 60-second window per client IP
- Storage: In-memory; each instance has its own rate-limit counter
- Suitable for: Single-instance deployments, development, testing

**With Redis (shared state):**
- Rate limit: 10 requests per 60-second window per client IP (same limit, shared across instances)
- Storage: Redis; all backend instances query the same counter
- Suitable for: Multi-replica deployments, load-balanced production setups

See the [Configuration](./configuration#redis-rate-limiter) guide for details on enabling Redis-backed rate limiting.
