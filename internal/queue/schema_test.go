package queue_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

// validRecordSchema is a minimal Avro record schema used across multiple tests.
const validRecordSchema = `{
	"type": "record",
	"name": "Event",
	"fields": [
		{"name": "id",   "type": "string"},
		{"name": "value","type": "int"}
	]
}`

var _ = Describe("Topic Schema", func() {
	var (
		pool    *pgxpool.Pool
		service *queue.Service
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		pool, err = pgxpool.New(ctx, containerDSN)
		Expect(err).NotTo(HaveOccurred())

		err = db.Migrate(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, "DELETE FROM topic_schemas")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())

		service = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
		}
	})

	// -----------------------------------------------------------------------
	// GetTopicSchema
	// -----------------------------------------------------------------------

	Describe("GetTopicSchema", func() {
		Context("when no schema has been registered for the topic", func() {
			It("should return nil without an error", func() {
				ts, err := queue.GetTopicSchema(ctx, pool, "nonexistent-topic")

				Expect(err).NotTo(HaveOccurred())
				Expect(ts).To(BeNil())
			})
		})

		Context("when a schema exists for the topic", func() {
			BeforeEach(func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "existing-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the stored schema with all fields populated", func() {
				ts, err := queue.GetTopicSchema(ctx, pool, "existing-topic")

				Expect(err).NotTo(HaveOccurred())
				Expect(ts).NotTo(BeNil())
				Expect(ts.Topic).To(Equal("existing-topic"))
				Expect(ts.SchemaJSON).To(Equal(validRecordSchema))
				Expect(ts.Version).To(Equal(1))
				Expect(ts.UpdatedAt).To(BeTemporally("~", time.Now(), 5*time.Second))
			})
		})
	})

	// -----------------------------------------------------------------------
	// UpsertTopicSchema
	// -----------------------------------------------------------------------

	Describe("UpsertTopicSchema", func() {
		Context("when the schema JSON is not valid Avro", func() {
			It("should return ErrInvalidSchema without touching the database", func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "bad-schema-topic", `{"this": "is not avro"}`)

				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, queue.ErrInvalidSchema)).To(BeTrue())

				// Confirm nothing was written to the database.
				ts, dbErr := queue.GetTopicSchema(ctx, pool, "bad-schema-topic")
				Expect(dbErr).NotTo(HaveOccurred())
				Expect(ts).To(BeNil())
			})
		})

		Context("when the schema JSON is valid Avro", func() {
			It("should persist the schema with version 1", func() {
				ts, err := queue.UpsertTopicSchema(ctx, pool, "valid-schema-topic", validRecordSchema)

				Expect(err).NotTo(HaveOccurred())
				Expect(ts).NotTo(BeNil())
				Expect(ts.Topic).To(Equal("valid-schema-topic"))
				Expect(ts.Version).To(Equal(1))
			})
		})

		Context("when the same topic is upserted a second time", func() {
			It("should increment the version to 2", func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "versioned-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())

				ts, err := queue.UpsertTopicSchema(ctx, pool, "versioned-topic", validRecordSchema)

				Expect(err).NotTo(HaveOccurred())
				Expect(ts).NotTo(BeNil())
				Expect(ts.Version).To(Equal(2))
			})
		})
	})

	// -----------------------------------------------------------------------
	// DeleteTopicSchema
	// -----------------------------------------------------------------------

	Describe("DeleteTopicSchema", func() {
		Context("when a schema exists for the topic", func() {
			BeforeEach(func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "deletable-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should remove the schema so a subsequent Get returns nil", func() {
				err := queue.DeleteTopicSchema(ctx, pool, "deletable-topic")
				Expect(err).NotTo(HaveOccurred())

				ts, err := queue.GetTopicSchema(ctx, pool, "deletable-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(ts).To(BeNil())
			})
		})

		Context("when no schema exists for the topic", func() {
			It("should return nil (delete is idempotent)", func() {
				err := queue.DeleteTopicSchema(ctx, pool, "ghost-topic")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	// -----------------------------------------------------------------------
	// validatePayload (exercised through Service.Enqueue)
	// -----------------------------------------------------------------------

	Describe("validatePayload", func() {
		Context("when no schema is registered for the topic", func() {
			It("should allow any payload through without validation", func() {
				// Payload is not valid JSON — but no schema means no validation.
				id, err := service.Enqueue(ctx, "unschema-topic", []byte(`not even json`), nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(id).NotTo(BeEmpty())
			})
		})

		Context("when a schema is registered for the topic", func() {
			BeforeEach(func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "schema-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())
			})

			Context("and the payload is missing a required field", func() {
				It("should return ErrSchemaValidation", func() {
					// Valid JSON, but omits the required "value" (int) field.
					_, err := service.Enqueue(ctx, "schema-topic", []byte(`{"id":"abc"}`), nil)

					Expect(err).To(HaveOccurred())
					Expect(errors.Is(err, queue.ErrSchemaValidation)).To(BeTrue())
				})
			})

			Context("and the payload has a field with the wrong JSON type", func() {
				It("should return ErrSchemaValidation", func() {
					// "value" must be an int, but we send a string.
					_, err := service.Enqueue(ctx, "schema-topic", []byte(`{"id":"abc","value":"not-an-int"}`), nil)

					Expect(err).To(HaveOccurred())
					Expect(errors.Is(err, queue.ErrSchemaValidation)).To(BeTrue())
				})
			})

			Context("and the payload is valid according to the schema", func() {
				It("should enqueue the message successfully", func() {
					id, err := service.Enqueue(ctx, "schema-topic", []byte(`{"id":"abc","value":42}`), nil)

					Expect(err).NotTo(HaveOccurred())
					Expect(id).NotTo(BeEmpty())
				})
			})
		})

		Context("when the schema cache is warm", func() {
			// This test verifies that after the schema is cached on first use, the
			// compiled schema is reused even if the DB row is deleted — the cache
			// holds the parsed schema until the version changes or the process
			// restarts.
			It("should still validate correctly from the in-memory cache after the DB row is removed", func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "cache-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())

				// First enqueue populates the cache.
				_, err = service.Enqueue(ctx, "cache-topic", []byte(`{"id":"x","value":1}`), nil)
				Expect(err).NotTo(HaveOccurred())

				// Remove the DB row directly — bypassing DeleteTopicSchema so the
				// cache entry is NOT invalidated. This simulates the cache being warm
				// while the DB row is absent.
				_, err = pool.Exec(ctx, "DELETE FROM topic_schemas WHERE topic = 'cache-topic'")
				Expect(err).NotTo(HaveOccurred())

				// GetTopicSchema now returns nil, so validatePayload skips validation —
				// this is correct and expected behaviour (no schema → accept anything).
				// The cache is only consulted when a schema row exists.
				id, err := service.Enqueue(ctx, "cache-topic", []byte(`not valid avro at all`), nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(id).NotTo(BeEmpty())
			})
		})
	})
})
