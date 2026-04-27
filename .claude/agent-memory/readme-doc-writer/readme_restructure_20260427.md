---
name: README Restructure (April 2026)
description: Complete rewrite of README.md for clarity, navigation, and compelling messaging
type: project
---

Completed a major README.md restructure on 2026-04-27:

**Changes made:**
1. **New introduction** — Replaced generic feature list with compelling tagline + 3-4 sentence pitch selling the PostgreSQL-only, no-separate-broker positioning
2. **New "Why queue-ti?" section** — 5 bullet points highlighting concrete differentiators (Postgres-only, at-least-once delivery, built-in DLQ, observability, admin UI, per-topic config, Avro schemas, Go client)
3. **Table of Contents** — Added fully linked TOC covering all ## sections with correct anchor links
4. **Logical reading order** — Reorganized from organic growth to: Hook → Quick Start → Client Lib → Config → Auth → Schemas → Queue Mechanics → Architecture → API Reference → Observability → Tests → Performance → Dev Workflow → Project Structure → Deployment → Release → Troubleshooting → Contributing
5. **Tighter writing** — Consolidated verbose prose while keeping all technical content (endpoints, tables, code blocks, curl examples, all intact)
6. **Removed redundancy** — Eliminated repeated configuration explanations and consolidated related sections

**Line count:**
- Original: 1,896 lines
- Rewritten: 1,521 lines
- Reduction: 20% through clarity, not deletion

**No content dropped:**
- All 4 gRPC methods (Enqueue, Dequeue, Ack, Nack) with full protobuf definitions
- All 20+ HTTP API endpoints documented
- All environment variable tables
- All code examples and curl commands
- All metrics definitions
- All benchmarking and load test guidance
- All deployment and release instructions
- All configuration reference tables
- All authentication endpoints and grant examples

**Key architectural facts preserved:**
- Single `messages` table with composite index `(topic, status, visibility_timeout, created_at)`
- `FOR UPDATE SKIP LOCKED` dequeue strategy
- At-least-once delivery with visibility timeouts
- DLQ automatic promotion and requeue behavior
- JWT auth with per-topic grants and role-based access
- Avro schema validation with caching
- Per-topic config overrides (max_retries, TTL, max_depth)

**Verified:**
- TOC anchor links match actual headings (all lowercase, hyphenated)
- No broken cross-references
- Frontend/backend layers still clearly documented
- Queue mechanics section moved up (key concept users need early)
- Admin UI features section still complete

This is the authoritative restructured README going forward.
