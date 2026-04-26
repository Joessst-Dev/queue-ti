---
name: JWT Authentication Feature Documentation
description: JWT-based authentication with user accounts, grants, and admin UI integration documented in README
type: project
---

JWT-based authentication and user management feature has been documented in the README with comprehensive coverage of:

**What was added:**
- JWT authentication (HS256) replacing basic auth as the primary auth mechanism
- User accounts table with username, password_hash, is_admin flag
- Per-user grants system with topic patterns (supports *, prefix globs, exact names) and actions (read, write, admin)
- Grant enforcement on queue operations: write required for enqueue/dequeue/ack/nack/requeue, read for list/stats
- Default admin user seeded on first startup from config.username/password

**Documented endpoints (NEW):**
- POST /api/auth/login — return JWT token (15-min expiry, HS256)
- POST /api/auth/refresh — refresh JWT token
- GET/POST/PUT/DELETE /api/users — user management (admin-only)
- GET /api/users/:id/grants — list grants for user (admin-only)
- POST /api/users/:id/grants — create grant (admin-only)
- DELETE /api/users/:id/grants/:grant_id — delete grant (admin-only)

**Documented config keys:**
- auth.enabled — boolean, false by default
- auth.jwt_secret — required when auth.enabled=true, HS256 signing key
- auth.username/auth.password — default admin user, seeded on first boot
- QUEUETI_AUTH_JWT_SECRET env var — overrides jwt_secret

**Architecture notes:**
- Admin UI stores tokens in sessionStorage, uses Bearer token in Authorization header
- Admin users bypass grant checks, regular users subject to per-topic grants
- Grant topic patterns: * (all), orders.* (prefix), orders (exact)
- Server fails fast if auth.enabled=true and jwt_secret is empty

**Docker Compose:**
- Added QUEUETI_AUTH_JWT_SECRET env var to docker-compose.yaml example

**Sections updated:**
1. Features list — replaced "basic auth" with "JWT authentication"
2. Architecture HTTP endpoints — added 7 new auth/user/grant endpoints
3. NEW: Authentication & User Management section (comprehensive, ~700 lines)
4. Configuration section — added jwt_secret, updated env var table
5. Environment Variables section — added QUEUETI_AUTH_JWT_SECRET

Why this is important: JWT auth is stateless, scalable, and supports granular per-user/per-topic access control which basic auth could not provide. This is critical for multi-team or SaaS deployments where topic isolation is a security boundary.
