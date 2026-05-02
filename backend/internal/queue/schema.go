package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hamba/avro/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Joessst-Dev/queue-ti/internal/broadcast"
)

var (
	ErrSchemaValidation = errors.New("payload does not match topic schema")
	ErrInvalidSchema    = errors.New("invalid avro schema")
)

// TopicSchema holds the registered Avro schema for a single topic.
type TopicSchema struct {
	Topic      string
	SchemaJSON string
	Version    int
	UpdatedAt  time.Time
}

// cachedSchema pairs a compiled avro.Schema with the database version it was
// parsed from, so we can detect when an upsert has bumped the version and
// re-parse accordingly.
type cachedSchema struct {
	schema  avro.Schema
	version int
}

// schemaCache is a process-level, lock-free cache of compiled Avro schemas
// keyed by topic name.
type schemaCache struct {
	m sync.Map
}

// globalSchemaCache is the single instance shared across all Service calls.
// It is invalidated (entry deleted) on every UpsertTopicSchema / DeleteTopicSchema.
var globalSchemaCache schemaCache

// GetTopicSchema returns the registered schema for the given topic, or nil
// when no schema has been registered. It returns an error only on database
// failure.
func GetTopicSchema(ctx context.Context, pool *pgxpool.Pool, topic string) (*TopicSchema, error) {
	var ts TopicSchema
	err := pool.QueryRow(ctx,
		`SELECT topic, schema_json, version, updated_at FROM topic_schemas WHERE topic = $1`,
		topic,
	).Scan(&ts.Topic, &ts.SchemaJSON, &ts.Version, &ts.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get topic schema: %w", err)
	}
	return &ts, nil
}

// ListTopicSchemas returns all registered schemas ordered by topic name.
func ListTopicSchemas(ctx context.Context, pool *pgxpool.Pool) ([]TopicSchema, error) {
	rows, err := pool.Query(ctx,
		`SELECT topic, schema_json, version, updated_at FROM topic_schemas ORDER BY topic`)
	if err != nil {
		return nil, fmt.Errorf("list topic schemas: %w", err)
	}
	defer rows.Close()

	var result []TopicSchema
	for rows.Next() {
		var ts TopicSchema
		if err := rows.Scan(&ts.Topic, &ts.SchemaJSON, &ts.Version, &ts.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list topic schemas scan: %w", err)
		}
		result = append(result, ts)
	}
	return result, rows.Err()
}

// UpsertTopicSchema registers or replaces the Avro schema for the given topic.
// It validates the schema before persisting and returns ErrInvalidSchema (wrapped)
// when the JSON is not a valid Avro schema. On a successful re-upsert the
// version column is incremented automatically by the database.
func UpsertTopicSchema(ctx context.Context, pool *pgxpool.Pool, topic, schemaJSON string) (*TopicSchema, error) {
	if _, err := avro.Parse(schemaJSON); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidSchema, err)
	}

	// Invalidate the in-process cache so the next enqueue re-parses from the DB.
	globalSchemaCache.m.Delete(topic)

	var ts TopicSchema
	err := pool.QueryRow(ctx, `
		INSERT INTO topic_schemas (topic, schema_json)
		VALUES ($1, $2)
		ON CONFLICT (topic) DO UPDATE
		  SET schema_json = EXCLUDED.schema_json,
		      version     = topic_schemas.version + 1,
		      updated_at  = now()
		RETURNING topic, schema_json, version, updated_at`,
		topic, schemaJSON,
	).Scan(&ts.Topic, &ts.SchemaJSON, &ts.Version, &ts.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert topic schema: %w", err)
	}
	return &ts, nil
}

// DeleteTopicSchema removes the schema for the given topic. It is a no-op
// (no error) when no schema was registered.
func DeleteTopicSchema(ctx context.Context, pool *pgxpool.Pool, topic string) error {
	globalSchemaCache.m.Delete(topic)
	if _, err := pool.Exec(ctx, `DELETE FROM topic_schemas WHERE topic = $1`, topic); err != nil {
		return fmt.Errorf("delete topic schema: %w", err)
	}
	return nil
}

func (s *Service) UpsertTopicSchemaAndNotify(ctx context.Context, topic, schemaJSON string) (*TopicSchema, error) {
	ts, err := UpsertTopicSchema(ctx, s.pool, topic, schemaJSON)
	if err != nil {
		return nil, err
	}
	if err := s.broadcaster.Publish(ctx, broadcast.ChannelSchemaChanged, topic); err != nil {
		slog.Warn("broadcast schema change failed", "topic", topic, "error", err)
	}
	return ts, nil
}

func (s *Service) DeleteTopicSchemaAndNotify(ctx context.Context, topic string) error {
	if err := DeleteTopicSchema(ctx, s.pool, topic); err != nil {
		return err
	}
	if err := s.broadcaster.Publish(ctx, broadcast.ChannelSchemaChanged, topic); err != nil {
		slog.Warn("broadcast schema delete failed", "topic", topic, "error", err)
	}
	return nil
}

// validatePayload checks the payload against the topic's registered Avro schema.
// It returns nil when no schema is registered for the topic (validation is
// optional). It returns ErrSchemaValidation (wrapped) when the payload does not
// conform to the schema, and ErrInvalidSchema (wrapped) when the stored schema
// itself cannot be parsed.
//
// Payloads are treated as JSON objects. Validation checks that:
//   - The payload is valid JSON.
//   - For a record schema, every field that has no default is present.
//   - Every present field's JSON value is compatible with its Avro type.
//
// Compiled schemas are cached in memory keyed by (topic, version), so
// avro.Parse is only invoked when the schema version changes.
func (s *Service) validatePayload(ctx context.Context, topic string, payload []byte) error {
	ts, err := GetTopicSchema(ctx, s.pool, topic)
	if err != nil {
		return err
	}
	if ts == nil {
		return nil // no schema registered — accept anything
	}

	compiled, err := s.resolveSchema(topic, ts)
	if err != nil {
		return err
	}

	if err := validateJSONAgainstAvro(compiled, payload); err != nil {
		return fmt.Errorf("%w: %s", ErrSchemaValidation, err)
	}
	return nil
}

// resolveSchema returns the compiled avro.Schema for the given topic, using the
// in-memory cache when the cached version matches the database version.
func (s *Service) resolveSchema(topic string, ts *TopicSchema) (avro.Schema, error) {
	if v, ok := globalSchemaCache.m.Load(topic); ok {
		entry := v.(cachedSchema)
		if entry.version == ts.Version {
			return entry.schema, nil
		}
	}

	compiled, err := avro.Parse(ts.SchemaJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidSchema, err)
	}
	globalSchemaCache.m.Store(topic, cachedSchema{schema: compiled, version: ts.Version})
	return compiled, nil
}

// validateJSONAgainstAvro validates a JSON-encoded payload against an Avro
// schema. Only record schemas are inspected field-by-field; for all other
// schema types the payload just needs to be valid JSON.
func validateJSONAgainstAvro(schema avro.Schema, payload []byte) error {
	rec, ok := schema.(*avro.RecordSchema)
	if !ok {
		// For non-record schemas (primitives, arrays, maps, …) we only require
		// the payload to be parseable JSON.
		var v any
		return json.Unmarshal(payload, &v)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(payload, &doc); err != nil {
		return fmt.Errorf("payload is not a valid JSON object: %w", err)
	}

	for _, field := range rec.Fields() {
		raw, present := doc[field.Name()]
		if !present {
			// A field with a default is optional in the payload.
			if field.HasDefault() {
				continue
			}
			return fmt.Errorf("missing required field %q", field.Name())
		}
		if err := validateJSONValueAgainstAvro(field.Type(), raw); err != nil {
			return fmt.Errorf("field %q: %w", field.Name(), err)
		}
	}
	return nil
}

// validateJSONValueAgainstAvro performs a best-effort type check of a single
// JSON value against an Avro schema type. It returns an error when the JSON
// kind is clearly incompatible with the Avro type (e.g. a string where an int
// is required). Unions are accepted as long as the value matches at least one
// branch.
func validateJSONValueAgainstAvro(schema avro.Schema, raw json.RawMessage) error {
	// Unwrap null literal early — valid for nullable unions.
	if string(raw) == "null" {
		if schema.Type() == avro.Null {
			return nil
		}
		if u, ok := schema.(*avro.UnionSchema); ok {
			for _, t := range u.Types() {
				if t.Type() == avro.Null {
					return nil
				}
			}
		}
		return fmt.Errorf("unexpected null for non-nullable type %s", schema.Type())
	}

	switch schema.Type() {
	case avro.String:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("expected string, got %s", kindOf(raw))
		}
	case avro.Int, avro.Long:
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return fmt.Errorf("expected integer, got %s", kindOf(raw))
		}
		if _, err := n.Int64(); err != nil {
			return fmt.Errorf("expected integer, got %s", kindOf(raw))
		}
	case avro.Float, avro.Double:
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return fmt.Errorf("expected number, got %s", kindOf(raw))
		}
	case avro.Boolean:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return fmt.Errorf("expected boolean, got %s", kindOf(raw))
		}
	case avro.Record:
		if err := validateJSONAgainstAvro(schema, raw); err != nil {
			return err
		}
	case avro.Union:
		u := schema.(*avro.UnionSchema)
		for _, branch := range u.Types() {
			if validateJSONValueAgainstAvro(branch, raw) == nil {
				return nil
			}
		}
		return fmt.Errorf("value does not match any branch of union")
	// Bytes, fixed, enum, array, map: accept any JSON that parses; deep
	// validation of those types is out of scope for a structural check.
	default:
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("invalid JSON value: %w", err)
		}
	}
	return nil
}

// kindOf returns a human-readable description of the first byte of a JSON value,
// used for error messages.
func kindOf(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "empty"
	}
	switch raw[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	default:
		return "number"
	}
}
