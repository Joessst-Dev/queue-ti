# Avro Schema Validation

Topics can have an optional Avro schema registered. When a schema is registered for a topic, all `Enqueue` calls validate the JSON payload against that schema before storing the message. Topics without a registered schema accept any payload.

## How It Works

- **Schema registration**: Register an Avro schema for a topic via `PUT /api/topic-schemas/:topic`. The schema must be valid Avro JSON; invalid schemas are rejected with HTTP 400.
- **Validation at enqueue**: When a message is enqueued to a topic with a schema, the payload is validated as JSON and checked against the schema structure. If the payload does not conform, the enqueue fails with HTTP 422.
- **No schema = no validation**: Topics without a registered schema accept any payload. Existing messages are unaffected when a schema is added, updated, or removed.
- **Performance**: Parsed Avro schemas are cached in memory per topic. The cache automatically invalidates when a schema is updated or deleted.

## Validation Rules

For record schemas (the most common Avro type):
- Every required field (fields with no default value) must be present in the JSON payload
- Every present field must have a value compatible with its Avro type
- Optional fields (fields with a default value) may be omitted from the payload
- For other Avro types (primitives, arrays, maps, unions), the payload must be valid JSON and the top-level type must be compatible

## Schema Registration Endpoints

### GET /api/topic-schemas

Lists all registered schemas.

```bash
curl -u admin:secret http://localhost:8080/api/topic-schemas
```

**Response:**

```json
{
  "items": [
    {
      "topic": "orders",
      "schema_json": "{\"type\":\"record\",\"name\":\"Order\",\"fields\":[{\"name\":\"id\",\"type\":\"int\"},{\"name\":\"total\",\"type\":\"float\"}]}",
      "version": 1,
      "updated_at": "2025-04-25T12:00:00Z"
    }
  ]
}
```

### PUT /api/topic-schemas/:topic

Registers or updates an Avro schema for a topic. If a schema already exists, the version is incremented.

```bash
curl -u admin:secret -X PUT http://localhost:8080/api/topic-schemas/orders \
  -H "Content-Type: application/json" \
  -d '{
    "schema_json": "{\"type\":\"record\",\"name\":\"Order\",\"fields\":[{\"name\":\"order_id\",\"type\":\"int\"},{\"name\":\"customer_email\",\"type\":\"string\"},{\"name\":\"amount\",\"type\":\"double\"}]}"
  }'
```

**Response:** HTTP 200 OK

### GET /api/topic-schemas/:topic

Fetches the schema registered for a specific topic.

```bash
curl -u admin:secret http://localhost:8080/api/topic-schemas/orders
```

**Response:** HTTP 200 OK or HTTP 404 if no schema is registered

### DELETE /api/topic-schemas/:topic

Removes the registered schema for a topic. Existing messages are unaffected.

```bash
curl -u admin:secret -X DELETE http://localhost:8080/api/topic-schemas/orders
```

**Response:** HTTP 204 No Content

## Validation Errors

When a payload fails validation, the error includes details about what went wrong:

```json
{"error": "payload does not match topic schema: missing required field \"order_id\""}
```

Common validation error messages:
- `missing required field "fieldname"` — A required field is absent from the payload
- `field "fieldname": expected string, got number` — A field has the wrong JSON type
- `payload is not a valid JSON object` — The payload is not valid JSON or is not an object for a record schema
