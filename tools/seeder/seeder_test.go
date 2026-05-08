package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"

	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// silentLogger discards all log output so test runs stay quiet.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recordedRequest captures a single inbound HTTP call for later assertions.
type recordedRequest struct {
	method string
	path   string
	body   []byte
}

var _ = Describe("SeedFile.validate", func() {
	Context("when every entry has a non-empty topic", func() {
		It("returns nil", func() {
			seed := &SeedFile{
				TopicConfigs: []queueti.TopicConfig{{Topic: "orders"}},
				TopicSchemas: []TopicSchemaEntry{{Topic: "orders", Schema: `{"type":"string"}`}},
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing"}},
				},
			}

			Expect(seed.validate()).To(Succeed())
		})
	})

	Context("when a topic config has an empty topic", func() {
		It("returns an error naming the index", func() {
			seed := &SeedFile{
				TopicConfigs: []queueti.TopicConfig{
					{Topic: "orders"},
					{Topic: ""},
				},
			}

			err := seed.validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("topic_configs[1]"))
			Expect(err.Error()).To(ContainSubstring("topic must not be empty"))
		})
	})

	Context("when a topic schema has an empty topic", func() {
		It("returns an error naming the index", func() {
			seed := &SeedFile{
				TopicSchemas: []TopicSchemaEntry{
					{Topic: "", Schema: `{"type":"string"}`},
				},
			}

			err := seed.validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("topic_schemas[0]"))
			Expect(err.Error()).To(ContainSubstring("topic must not be empty"))
		})
	})

	Context("when a consumer group entry has an empty topic", func() {
		It("returns an error naming the index", func() {
			seed := &SeedFile{
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing"}},
					{Topic: "shipments", Groups: []string{"warehouse"}},
					{Topic: "", Groups: []string{"orphan"}},
				},
			}

			err := seed.validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("consumer_groups[2]"))
			Expect(err.Error()).To(ContainSubstring("topic must not be empty"))
		})
	})

	Context("when all sections are populated and valid", func() {
		It("returns nil", func() {
			seed := &SeedFile{
				TopicConfigs: []queueti.TopicConfig{
					{Topic: "orders"},
					{Topic: "shipments"},
				},
				TopicSchemas: []TopicSchemaEntry{
					{Topic: "orders", Schema: `{"type":"string"}`},
					{Topic: "shipments", Schema: `{"type":"string"}`},
				},
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing", "invoicing"}},
					{Topic: "shipments", Groups: []string{"warehouse"}},
				},
			}

			Expect(seed.validate()).To(Succeed())
		})
	})
})

var _ = Describe("loadSeed", func() {
	Context("when the file does not exist", func() {
		It("returns a read error", func() {
			_, err := loadSeed("/no/such/file.json")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("read seed file"))
		})
	})

	Context("when the file contains invalid JSON", func() {
		It("returns a parse error", func() {
			path := filepath.Join(GinkgoT().TempDir(), "bad.json")
			Expect(os.WriteFile(path, []byte("not json"), 0600)).To(Succeed())

			_, err := loadSeed(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse seed file"))
		})
	})

	Context("when the file fails validation", func() {
		It("returns a validation error", func() {
			path := filepath.Join(GinkgoT().TempDir(), "invalid.json")
			Expect(os.WriteFile(path, []byte(`{"topic_configs":[{"topic":""}]}`), 0600)).To(Succeed())

			_, err := loadSeed(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid seed file"))
		})
	})

	Context("when the file is valid", func() {
		It("returns the parsed SeedFile", func() {
			path := filepath.Join(GinkgoT().TempDir(), "seed.json")
			Expect(os.WriteFile(path, []byte(`{"topic_configs":[{"topic":"orders"}]}`), 0600)).To(Succeed())

			seed, err := loadSeed(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(seed.TopicConfigs).To(HaveLen(1))
			Expect(seed.TopicConfigs[0].Topic).To(Equal("orders"))
		})
	})
})

var _ = Describe("resolveToken", func() {
	Context("when --token is provided", func() {
		It("returns it immediately without any HTTP calls", func() {
			tok, err := resolveToken(context.Background(), "http://unused", "static-token", "", "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(tok).To(Equal("static-token"))
		})
	})

	Context("when neither token nor username is set", func() {
		It("returns an empty token", func() {
			tok, err := resolveToken(context.Background(), "http://unused", "", "", "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(tok).To(BeEmpty())
		})
	})

	Context("when username is set but no password is available", func() {
		It("returns an error", func() {
			_, err := resolveToken(context.Background(), "http://unused", "", "admin", "", &http.Client{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--password or SEEDER_PASSWORD"))
		})
	})

	Context("when username and password flag are provided", func() {
		It("calls NewAuth and returns the token", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/auth/status":
					Expect(json.NewEncoder(w).Encode(map[string]any{"auth_required": true})).To(Succeed())
				case "/api/auth/login":
					Expect(json.NewEncoder(w).Encode(map[string]any{"token": "jwt-from-login"})).To(Succeed())
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			tok, err := resolveToken(context.Background(), srv.URL, "", "admin", "secret", &http.Client{})
			Expect(err).NotTo(HaveOccurred())
			Expect(tok).To(Equal("jwt-from-login"))
		})
	})

	Context("when SEEDER_PASSWORD env var is set and --password is not", func() {
		It("uses the env var for login", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/auth/status":
					Expect(json.NewEncoder(w).Encode(map[string]any{"auth_required": true})).To(Succeed())
				case "/api/auth/login":
					Expect(json.NewEncoder(w).Encode(map[string]any{"token": "env-token"})).To(Succeed())
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			GinkgoT().Setenv("SEEDER_PASSWORD", "env-secret")
			tok, err := resolveToken(context.Background(), srv.URL, "", "admin", "", &http.Client{})
			Expect(err).NotTo(HaveOccurred())
			Expect(tok).To(Equal("env-token"))
		})
	})
})

var _ = Describe("Seeder.Apply", func() {
	var (
		ctx context.Context
		log *slog.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		log = silentLogger()
	})

	Describe("dry-run mode", func() {
		It("makes zero HTTP calls regardless of seed contents", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				Fail("dry-run should not perform any HTTP request, got " + r.Method + " " + r.URL.Path)
			}))
			defer srv.Close()

			admin := queueti.NewAdminClient(srv.URL)
			seeder := newSeeder(admin, true, log)

			seed := &SeedFile{
				TopicConfigs: []queueti.TopicConfig{{Topic: "orders"}},
				TopicSchemas: []TopicSchemaEntry{{Topic: "orders", Schema: `{"type":"string"}`}},
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing", "invoicing"}},
				},
			}

			Expect(seeder.Apply(ctx, seed)).To(Succeed())
		})

	})

	Describe("topic configs", func() {
		Context("when the server accepts every PUT", func() {
			It("PUTs each config to /api/topic-configs/{topic}", func() {
				var (
					mu       sync.Mutex
					captured []recordedRequest
				)

				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					body, _ := io.ReadAll(r.Body)
					mu.Lock()
					captured = append(captured, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					Expect(json.NewEncoder(w).Encode(map[string]any{"topic": "x"})).To(Succeed())
				}))
				defer srv.Close()

				admin := queueti.NewAdminClient(srv.URL)
				seeder := newSeeder(admin, false, log)

				seed := &SeedFile{
					TopicConfigs: []queueti.TopicConfig{
						{Topic: "orders"},
						{Topic: "shipments"},
					},
				}

				Expect(seeder.Apply(ctx, seed)).To(Succeed())

				Expect(captured).To(HaveLen(2))
				Expect(captured[0].method).To(Equal(http.MethodPut))
				Expect(captured[0].path).To(Equal("/api/topic-configs/orders"))
				Expect(captured[1].method).To(Equal(http.MethodPut))
				Expect(captured[1].path).To(Equal("/api/topic-configs/shipments"))
			})
		})

		Context("when the server responds non-2xx", func() {
			It("returns a wrapped error naming the topic", func() {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "boom", http.StatusInternalServerError)
				}))
				defer srv.Close()

				admin := queueti.NewAdminClient(srv.URL)
				seeder := newSeeder(admin, false, log)

				seed := &SeedFile{
					TopicConfigs: []queueti.TopicConfig{{Topic: "orders"}},
				}

				err := seeder.Apply(ctx, seed)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`topic config "orders"`))
			})
		})
	})

	Describe("topic schemas", func() {
		Context("when the server accepts every PUT", func() {
			It("PUTs each schema to /api/topic-schemas/{topic}", func() {
				var (
					mu       sync.Mutex
					captured []recordedRequest
				)

				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					body, _ := io.ReadAll(r.Body)
					mu.Lock()
					captured = append(captured, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					Expect(json.NewEncoder(w).Encode(map[string]any{"topic": "x", "schema_json": "{}", "version": 1})).To(Succeed())
				}))
				defer srv.Close()

				admin := queueti.NewAdminClient(srv.URL)
				seeder := newSeeder(admin, false, log)

				seed := &SeedFile{
					TopicSchemas: []TopicSchemaEntry{
						{Topic: "orders", Schema: `{"type":"string"}`},
						{Topic: "shipments", Schema: `{"type":"int"}`},
					},
				}

				Expect(seeder.Apply(ctx, seed)).To(Succeed())

				Expect(captured).To(HaveLen(2))
				Expect(captured[0].method).To(Equal(http.MethodPut))
				Expect(captured[0].path).To(Equal("/api/topic-schemas/orders"))
				Expect(captured[1].method).To(Equal(http.MethodPut))
				Expect(captured[1].path).To(Equal("/api/topic-schemas/shipments"))

				// The body should carry the schema JSON the AdminClient sends.
				var body map[string]string
				Expect(json.Unmarshal(captured[0].body, &body)).To(Succeed())
				Expect(body["schema_json"]).To(Equal(`{"type":"string"}`))
			})
		})

		Context("when the server responds non-2xx", func() {
			It("returns a wrapped error naming the topic", func() {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "boom", http.StatusBadRequest)
				}))
				defer srv.Close()

				admin := queueti.NewAdminClient(srv.URL)
				seeder := newSeeder(admin, false, log)

				seed := &SeedFile{
					TopicSchemas: []TopicSchemaEntry{
						{Topic: "orders", Schema: `{"type":"string"}`},
					},
				}

				err := seeder.Apply(ctx, seed)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`topic schema "orders"`))
			})
		})
	})

	Describe("consumer groups", func() {
		// mux returns a fresh ServeMux + recorder for tests that need to
		// distinguish list/register calls per topic.
		type recorder struct {
			mu        sync.Mutex
			lists     map[string]int      // topic -> times listed
			registers map[string][]string // topic -> groups registered (in order)
			existing  map[string][]string // topic -> groups already on the server
			// registerStatus maps a topic to the HTTP status returned for every POST on that topic.
			// Only one status per topic is supported; use separate tests for per-group variation.
			registerStatus map[string]int
		}

		newRecorder := func() *recorder {
			return &recorder{
				lists:          make(map[string]int),
				registers:      make(map[string][]string),
				existing:       make(map[string][]string),
				registerStatus: make(map[string]int),
			}
		}

		buildServer := func(rec *recorder) *httptest.Server {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/topics/", func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				// Path shape: /api/topics/{topic}/consumer-groups
				path := r.URL.Path
				const prefix = "/api/topics/"
				const suffix = "/consumer-groups"
				if len(path) <= len(prefix)+len(suffix) || path[:len(prefix)] != prefix || path[len(path)-len(suffix):] != suffix {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				topic := path[len(prefix) : len(path)-len(suffix)]

				rec.mu.Lock()
				defer rec.mu.Unlock()

				switch r.Method {
				case http.MethodGet:
					rec.lists[topic]++
					w.Header().Set("Content-Type", "application/json")
					Expect(json.NewEncoder(w).Encode(map[string]any{"items": rec.existing[topic]})).To(Succeed())
				case http.MethodPost:
					var body map[string]string
					Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
					if status, ok := rec.registerStatus[topic]; ok {
						w.WriteHeader(status)
						return
					}
					rec.registers[topic] = append(rec.registers[topic], body["consumer_group"])
					w.WriteHeader(http.StatusCreated)
				default:
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			})
			return httptest.NewServer(mux)
		}

		It("skips entries with no groups and makes no HTTP calls for them", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				Fail("should not make any HTTP call for an empty groups entry, got " + r.Method + " " + r.URL.Path)
			}))
			defer srv.Close()

			admin := queueti.NewAdminClient(srv.URL)
			seeder := newSeeder(admin, false, log)

			seed := &SeedFile{
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: nil},
				},
			}

			Expect(seeder.Apply(ctx, seed)).To(Succeed())
		})

		It("lists existing groups before registering and registers only missing ones", func() {
			rec := newRecorder()
			rec.existing["orders"] = []string{"billing"} // already exists

			srv := buildServer(rec)
			defer srv.Close()

			admin := queueti.NewAdminClient(srv.URL)
			seeder := newSeeder(admin, false, log)

			seed := &SeedFile{
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing", "invoicing"}},
				},
			}

			Expect(seeder.Apply(ctx, seed)).To(Succeed())

			Expect(rec.lists["orders"]).To(Equal(1))
			Expect(rec.registers["orders"]).To(Equal([]string{"invoicing"}))
		})

		It("skips groups already in the existing list (no POST made)", func() {
			rec := newRecorder()
			rec.existing["orders"] = []string{"billing", "invoicing"}

			srv := buildServer(rec)
			defer srv.Close()

			admin := queueti.NewAdminClient(srv.URL)
			seeder := newSeeder(admin, false, log)

			seed := &SeedFile{
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing", "invoicing"}},
				},
			}

			Expect(seeder.Apply(ctx, seed)).To(Succeed())

			Expect(rec.lists["orders"]).To(Equal(1))
			Expect(rec.registers["orders"]).To(BeEmpty())
		})

		It("tolerates 409 Conflict from RegisterConsumerGroup without returning an error", func() {
			rec := newRecorder()
			rec.registerStatus["orders"] = http.StatusConflict

			srv := buildServer(rec)
			defer srv.Close()

			admin := queueti.NewAdminClient(srv.URL)
			seeder := newSeeder(admin, false, log)

			seed := &SeedFile{
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing"}},
				},
			}

			Expect(seeder.Apply(ctx, seed)).To(Succeed())
			Expect(rec.lists["orders"]).To(Equal(1))
		})

		It("returns an error when ListConsumerGroups fails", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusCreated)
			}))
			defer srv.Close()

			admin := queueti.NewAdminClient(srv.URL)
			seeder := newSeeder(admin, false, log)

			seed := &SeedFile{
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing"}},
				},
			}

			err := seeder.Apply(ctx, seed)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`list consumer groups for topic "orders"`))
		})

		It("returns an error when RegisterConsumerGroup fails with non-409", func() {
			rec := newRecorder()
			rec.registerStatus["orders"] = http.StatusInternalServerError

			srv := buildServer(rec)
			defer srv.Close()

			admin := queueti.NewAdminClient(srv.URL)
			seeder := newSeeder(admin, false, log)

			seed := &SeedFile{
				ConsumerGroups: []ConsumerGroupEntry{
					{Topic: "orders", Groups: []string{"billing"}},
				},
			}

			err := seeder.Apply(ctx, seed)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`register consumer group "billing" on topic "orders"`))
		})
	})
})
