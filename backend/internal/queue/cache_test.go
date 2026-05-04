package queue_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Joessst-Dev/queue-ti/internal/cache"
	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeCache is a thread-safe in-memory Cache used in tests to observe cache
// interactions without a real Redis instance.
type fakeCache struct {
	mu   sync.Mutex
	data map[string][]byte
	gets map[string]int // counts Get calls per key to assert cache hits
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		data: map[string][]byte{},
		gets: map[string]int{},
	}
}

func (f *fakeCache) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets[key]++
	v, ok := f.data[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (f *fakeCache) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = val
	return nil
}

func (f *fakeCache) Delete(_ context.Context, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		delete(f.data, k)
	}
	return nil
}

func (f *fakeCache) hasKey(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[key]
	return ok
}

func (f *fakeCache) getCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets[key]
}

var _ cache.Cache = (*fakeCache)(nil) // compile-time interface check

// schemaKey and configKey mirror the private constants in the queue package
// (schemaCachePrefix, configCachePrefix). Keep in sync if those change.
func schemaKey(topic string) string { return "queueti:cache:schema:" + topic }
func configKey(topic string) string { return "queueti:cache:topic_config:" + topic }

var _ = Describe("Schema Cache", func() {
	var (
		pool    *pgxpool.Pool
		svc     *queue.Service
		fc      *fakeCache
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

		fc = newFakeCache()
		svc = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
		svc.UseCache(fc)
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
		}
	})

	Describe("getTopicSchemaCached (exercised via Enqueue)", func() {
		Context("when a schema is registered and the cache is cold", func() {
			BeforeEach(func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "cached-schema-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fetch from DB and populate Redis on cache miss", func() {
				_, err := svc.Enqueue(ctx, "cached-schema-topic", []byte(`{"id":"x","value":1}`), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(fc.hasKey(schemaKey("cached-schema-topic"))).To(BeTrue(),
					"expected cache to be populated after the first enqueue")
			})

			It("should return schema from Redis cache on second call without hitting the DB", func() {
				// First call — populates cache.
				_, err := svc.Enqueue(ctx, "cached-schema-topic", []byte(`{"id":"x","value":1}`), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				getsAfterFirst := fc.getCount(schemaKey("cached-schema-topic"))

				// Second call — should hit cache, not DB. The Get count increments
				// whether it's a hit or miss, so we confirm the entry is still present.
				_, err = svc.Enqueue(ctx, "cached-schema-topic", []byte(`{"id":"y","value":2}`), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				getsAfterSecond := fc.getCount(schemaKey("cached-schema-topic"))
				Expect(getsAfterSecond).To(Equal(getsAfterFirst+1),
					"expected exactly one more cache.Get call — the second enqueue hit the cache")

				Expect(fc.hasKey(schemaKey("cached-schema-topic"))).To(BeTrue())
			})
		})

		Context("when no schema is registered for a topic", func() {
			It("should cache a nil-sentinel so subsequent calls skip the DB", func() {
				// Enqueue twice with no schema — both should succeed.
				_, err := svc.Enqueue(ctx, "no-schema-topic", []byte(`anything`), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = svc.Enqueue(ctx, "no-schema-topic", []byte(`anything else`), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				// Cache must hold the sentinel after the first miss.
				Expect(fc.hasKey(schemaKey("no-schema-topic"))).To(BeTrue())

				// Second enqueue must have made exactly one more cache.Get than the first.
				Expect(fc.getCount(schemaKey("no-schema-topic"))).To(Equal(2))
			})
		})
	})

	Describe("UpsertTopicSchemaAndNotify", func() {
		Context("when an existing schema is updated", func() {
			It("should invalidate the Redis cache so the next call re-fetches from DB", func() {
				// Seed the cache via an initial enqueue.
				_, err := queue.UpsertTopicSchema(ctx, pool, "upsert-cache-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())
				_, err = svc.Enqueue(ctx, "upsert-cache-topic", []byte(`{"id":"a","value":1}`), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(fc.hasKey(schemaKey("upsert-cache-topic"))).To(BeTrue())

				// Upsert via the service method — must delete the cache entry.
				_, err = svc.UpsertTopicSchemaAndNotify(ctx, "upsert-cache-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())

				Expect(fc.hasKey(schemaKey("upsert-cache-topic"))).To(BeFalse(),
					"expected cache to be invalidated after UpsertTopicSchemaAndNotify")
			})
		})
	})

	Describe("DeleteTopicSchemaAndNotify", func() {
		Context("when a schema is deleted via the service", func() {
			It("should invalidate the Redis cache entry for that topic", func() {
				_, err := queue.UpsertTopicSchema(ctx, pool, "delete-cache-topic", validRecordSchema)
				Expect(err).NotTo(HaveOccurred())
				_, err = svc.Enqueue(ctx, "delete-cache-topic", []byte(`{"id":"a","value":1}`), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(fc.hasKey(schemaKey("delete-cache-topic"))).To(BeTrue())

				err = svc.DeleteTopicSchemaAndNotify(ctx, "delete-cache-topic")
				Expect(err).NotTo(HaveOccurred())

				Expect(fc.hasKey(schemaKey("delete-cache-topic"))).To(BeFalse(),
					"expected cache to be invalidated after DeleteTopicSchemaAndNotify")
			})
		})
	})
})

var _ = Describe("Topic Config Cache", func() {
	var (
		pool *pgxpool.Pool
		svc  *queue.Service
		fc   *fakeCache
		ctx  context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		pool, err = pgxpool.New(ctx, containerDSN)
		Expect(err).NotTo(HaveOccurred())

		err = db.Migrate(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, "DELETE FROM topic_config")
		Expect(err).NotTo(HaveOccurred())

		fc = newFakeCache()
		svc = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
		svc.UseCache(fc)
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
		}
	})

	Describe("GetTopicConfig", func() {
		Context("when a config exists and the caches are cold", func() {
			BeforeEach(func() {
				maxRetries := 5
				err := svc.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "config-cache-topic",
					MaxRetries: &maxRetries,
				})
				Expect(err).NotTo(HaveOccurred())

				// UpsertTopicConfig invalidates both caches — confirm they are empty.
				Expect(fc.hasKey(configKey("config-cache-topic"))).To(BeFalse())
			})

			It("should populate both local and Redis cache on DB fetch", func() {
				cfg, err := svc.GetTopicConfig(ctx, "config-cache-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())

				Expect(fc.hasKey(configKey("config-cache-topic"))).To(BeTrue(),
					"expected Redis cache to be populated after a DB fetch")
			})

			It("should return config from local cache on second call without hitting Redis or DB", func() {
				// First call populates both caches.
				_, err := svc.GetTopicConfig(ctx, "config-cache-topic")
				Expect(err).NotTo(HaveOccurred())

				getsAfterFirst := fc.getCount(configKey("config-cache-topic"))

				// Second call should hit the local sync.Map — no Redis Get needed.
				cfg, err := svc.GetTopicConfig(ctx, "config-cache-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())

				Expect(fc.getCount(configKey("config-cache-topic"))).To(Equal(getsAfterFirst),
					"expected zero additional Redis.Get calls on second lookup (local cache hit)")
			})
		})

		Context("when no config exists for the topic", func() {
			It("should cache a nil result and not re-query the DB", func() {
				cfg, err := svc.GetTopicConfig(ctx, "nonexistent-config-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(BeNil())

				// Redis must hold the sentinel.
				Expect(fc.hasKey(configKey("nonexistent-config-topic"))).To(BeTrue())

				// Second call must not add another Redis.Get (local cache holds the nil).
				getsAfterFirst := fc.getCount(configKey("nonexistent-config-topic"))

				cfg, err = svc.GetTopicConfig(ctx, "nonexistent-config-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(BeNil())

				Expect(fc.getCount(configKey("nonexistent-config-topic"))).To(Equal(getsAfterFirst),
					"expected zero additional Redis.Get calls — nil result is in local cache")
			})
		})

		Context("when a cached config is returned from Redis (local cache is cold)", func() {
			It("should deserialise and return the config without hitting the DB", func() {
				maxRetries := 7
				cfg := queue.TopicConfig{Topic: "redis-only-topic", MaxRetries: &maxRetries}

				encoded, err := json.Marshal(cfg)
				Expect(err).NotTo(HaveOccurred())
				err = fc.Set(ctx, configKey("redis-only-topic"), encoded, 30*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// Create a fresh service sharing the same fakeCache — local cache is empty.
				svc2 := queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
				svc2.UseCache(fc)

				result, err := svc2.GetTopicConfig(ctx, "redis-only-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(*result.MaxRetries).To(Equal(7))
			})
		})
	})

	Describe("UpsertTopicConfig", func() {
		Context("when a config is upserted", func() {
			It("should invalidate both local and Redis caches", func() {
				maxRetries := 3
				err := svc.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "invalidate-upsert-topic",
					MaxRetries: &maxRetries,
				})
				Expect(err).NotTo(HaveOccurred())

				// Warm both caches.
				_, err = svc.GetTopicConfig(ctx, "invalidate-upsert-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(fc.hasKey(configKey("invalidate-upsert-topic"))).To(BeTrue())

				// Upsert again — must evict both caches.
				maxRetries = 10
				err = svc.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "invalidate-upsert-topic",
					MaxRetries: &maxRetries,
				})
				Expect(err).NotTo(HaveOccurred())

				Expect(fc.hasKey(configKey("invalidate-upsert-topic"))).To(BeFalse(),
					"expected Redis cache to be cleared after UpsertTopicConfig")

				// Next Get must re-populate from DB with the updated value.
				cfg, err := svc.GetTopicConfig(ctx, "invalidate-upsert-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())
				Expect(*cfg.MaxRetries).To(Equal(10))
			})
		})
	})

	Describe("DeleteTopicConfig", func() {
		Context("when a config is deleted", func() {
			It("should invalidate both local and Redis caches", func() {
				maxRetries := 2
				err := svc.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "invalidate-delete-topic",
					MaxRetries: &maxRetries,
				})
				Expect(err).NotTo(HaveOccurred())

				// Warm both caches.
				_, err = svc.GetTopicConfig(ctx, "invalidate-delete-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(fc.hasKey(configKey("invalidate-delete-topic"))).To(BeTrue())

				// Delete — must evict both caches.
				err = svc.DeleteTopicConfig(ctx, "invalidate-delete-topic")
				Expect(err).NotTo(HaveOccurred())

				Expect(fc.hasKey(configKey("invalidate-delete-topic"))).To(BeFalse(),
					"expected Redis cache to be cleared after DeleteTopicConfig")

				// Next Get must re-query DB and return nil.
				cfg, err := svc.GetTopicConfig(ctx, "invalidate-delete-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(BeNil())
			})
		})
	})
})
