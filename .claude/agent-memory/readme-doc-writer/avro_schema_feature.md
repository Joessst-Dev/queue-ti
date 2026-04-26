---
name: Avro Schema Validation Feature Documentation
description: Complete documentation of Avro schema validation feature including registration, validation rules, endpoints, errors, and examples
type: reference
---

## Avro Schema Validation Feature Documentation

The Avro schema validation feature has been fully documented in README.md. This memory records where and what was documented.

### Features Section (line 16)
Added to the top-level Features list:
- "Avro schema validation — Optional per-topic Avro schema registration; payloads are validated at enqueue time with compiled schemas cached in memory"

### New "Avro Schema Validation" Section (lines 214-372)
A new H2 section was added after Topic-Level Configuration Overrides and before Architecture, containing:

1. **Overview** (lines 214-216): Plain explanation that topics can have optional Avro schemas, and how they work
2. **How It Works** (lines 218-223): Four key bullets covering schema registration, validation at enqueue, no-schema behavior, and caching/invalidation
3. **Validation Rules** (lines 225-231): Rules for record schemas (most common) and other Avro types
4. **Schema Registration Endpoints** (lines 233-322): Four endpoints fully documented:
   - GET /api/topic-schemas — lists all registered schemas (line 235-255)
   - PUT /api/topic-schemas/:topic — register/update schema (line 257-290)
   - DELETE /api/topic-schemas/:topic — remove schema (line 292-303)
   - GET /api/topic-schemas/:topic — fetch single schema (line 305-323)
5. **Validation Errors** (lines 325-340): HTTP 422 with example, plus common error messages
6. **Example: Register and Use a Schema** (lines 342-372): Three curl examples showing register, valid enqueue, and invalid enqueue

### Architecture Section Updates
1. **Queue Mechanics** (line 446): Added bullet point documenting Avro schema validation in Queue Mechanics
2. **HTTP Server Endpoints** (lines 397-400): Added four topic-schemas endpoints to the ASCII tree:
   - GET /api/topic-schemas
   - PUT /api/topic-schemas/:topic
   - DELETE /api/topic-schemas/:topic
   - GET /api/topic-schemas/:topic

### Implementation Details Verified
- Error mapping: ErrSchemaValidation → HTTP 422 (Unprocessable Entity) in http.go, codes.InvalidArgument in grpc.go
- Error mapping: ErrInvalidSchema → HTTP 400 (Bad Request) in http.go
- Schema caching: In-memory sync.Map cache per topic, invalidated on upsert/delete
- Database: topic_schemas table with (topic, schema_json, version, created_at, updated_at)
- Validation: validatePayload() in queue/schema.go validates JSON against Avro schema before enqueue
- No-schema behavior: Returns nil when no schema registered (accept anything)

### Key Documentation Principles Applied
- Accurate error codes from actual implementation (verified in internal/server/http.go and internal/server/grpc.go)
- Performance note about in-memory caching with automatic invalidation
- Copy-paste-ready curl examples with realistic Avro schema
- Clear explanation of validation rules (record field requirements, type compatibility)
- Integration note: "Topics without a registered schema accept any payload"
- Endpoint organization: overview, then all four CRUD endpoints, then errors, then practical example

### Terminology Used
- "Avro schema" not "Avro format" or "schema"
- "payload validation" not "payload checking"
- "topic" not "queue" or "channel"
- "schema version" refers to the incremented version in topic_schemas table
- "compiled schemas" refers to parsed avro.Schema objects in memory cache
