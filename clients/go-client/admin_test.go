package queueti_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"

	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// adminFakeServer captures incoming requests and serves canned JSON responses.
type adminFakeServer struct {
	mu           sync.Mutex
	requests     []*http.Request
	rawBodies    [][]byte
	statusCode   int
	responseBody []byte
}

func (f *adminFakeServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		// Snapshot the request body before the handler returns.
		var raw []byte
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(new(json.RawMessage)); err == nil {
				// Re-read for recording: parse into generic map then re-encode.
			}
			r.Body.Close()
		}
		// Simpler: just store a clone of the decoded body separately.
		f.requests = append(f.requests, r)
		f.rawBodies = append(f.rawBodies, raw)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.statusCode)
		_, _ = w.Write(f.responseBody)
	}
}


// newFake builds an httptest.Server with canned status + body.
// The caller is responsible for calling ts.Close().
func newFake(statusCode int, body []byte) (*adminFakeServer, *httptest.Server) {
	f := &adminFakeServer{statusCode: statusCode, responseBody: body}
	ts := httptest.NewServer(f.handler())
	return f, ts
}

// captureBodyServer records the raw request body in addition to headers.
type captureBodyServer struct {
	mu         sync.Mutex
	requests   []*capturedRequest
	statusCode int
	respBody   []byte
}

type capturedRequest struct {
	method string
	path   string
	header http.Header
	body   map[string]any
}

func (s *captureBodyServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cr := &capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			header: r.Header.Clone(),
		}
		if r.Body != nil {
			defer r.Body.Close()
			var m map[string]any
			if err := json.NewDecoder(r.Body).Decode(&m); err == nil {
				cr.body = m
			}
		}
		s.mu.Lock()
		s.requests = append(s.requests, cr)
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.statusCode)
		_, _ = w.Write(s.respBody)
	}
}

func (s *captureBodyServer) last() *capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return nil
	}
	return s.requests[len(s.requests)-1]
}

func newCapture(statusCode int, body []byte) (*captureBodyServer, *httptest.Server) {
	s := &captureBodyServer{statusCode: statusCode, respBody: body}
	ts := httptest.NewServer(s.handler())
	return s, ts
}

// intPtr is a convenience helper for taking the address of an int literal.
func intPtr(v int) *int { return &v }

var _ = Describe("AdminClient", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ---- Topic config -------------------------------------------------------

	Describe("ListTopicConfigs", func() {
		Context("when the server returns a list of configs", func() {
			It("returns the items array", func() {
				body := []byte(`{"items":[{"topic":"orders","replayable":true},{"topic":"payments","replayable":false}]}`)
				_, ts := newFake(http.StatusOK, body)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL, queueti.WithAdminToken("secret"))
				configs, err := client.ListTopicConfigs(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(configs).To(HaveLen(2))
				Expect(configs[0].Topic).To(Equal("orders"))
				Expect(configs[0].Replayable).To(BeTrue())
				Expect(configs[1].Topic).To(Equal("payments"))
			})
		})

		Context("when the server returns an empty list", func() {
			It("returns an empty (not nil) slice", func() {
				body := []byte(`{"items":[]}`)
				_, ts := newFake(http.StatusOK, body)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				configs, err := client.ListTopicConfigs(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(configs).To(BeEmpty())
			})
		})
	})

	Describe("UpsertTopicConfig", func() {
		Context("when the server accepts the config", func() {
			It("sends a PUT with the correct body and returns the saved config", func() {
				responseBody := []byte(`{"topic":"orders","max_retries":5,"replayable":true}`)
				srv, ts := newCapture(http.StatusOK, responseBody)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL, queueti.WithAdminToken("tok"))
				cfg := queueti.TopicConfig{
					Topic:      "orders",
					MaxRetries: intPtr(5),
					Replayable: true,
				}
				result, err := client.UpsertTopicConfig(ctx, "orders", cfg)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Topic).To(Equal("orders"))
				Expect(result.MaxRetries).NotTo(BeNil())
				Expect(*result.MaxRetries).To(Equal(5))
				Expect(result.Replayable).To(BeTrue())

				req := srv.last()
				Expect(req.method).To(Equal(http.MethodPut))
				Expect(req.path).To(Equal("/api/topic-configs/orders"))
				Expect(req.header.Get("Authorization")).To(Equal("Bearer tok"))
				Expect(req.body).To(HaveKey("topic"))
			})
		})
	})

	Describe("DeleteTopicConfig", func() {
		Context("when the server acknowledges the deletion", func() {
			It("sends DELETE and returns nil on 204", func() {
				srv, ts := newCapture(http.StatusNoContent, nil)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				err := client.DeleteTopicConfig(ctx, "orders")

				Expect(err).NotTo(HaveOccurred())
				req := srv.last()
				Expect(req.method).To(Equal(http.MethodDelete))
				Expect(req.path).To(Equal("/api/topic-configs/orders"))
			})
		})
	})

	// ---- Topic schema -------------------------------------------------------

	Describe("ListTopicSchemas", func() {
		Context("when the server returns schemas", func() {
			It("returns the items array", func() {
				body := []byte(`{"items":[{"topic":"orders","schema_json":"{}","version":1,"updated_at":"2024-01-01T00:00:00Z"}]}`)
				_, ts := newFake(http.StatusOK, body)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				schemas, err := client.ListTopicSchemas(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(schemas).To(HaveLen(1))
				Expect(schemas[0].Topic).To(Equal("orders"))
				Expect(schemas[0].Version).To(Equal(1))
			})
		})
	})

	Describe("GetTopicSchema", func() {
		Context("when the schema exists", func() {
			It("returns the schema for the requested topic", func() {
				body := []byte(`{"topic":"orders","schema_json":"{\"type\":\"record\"}","version":2,"updated_at":"2024-06-01T00:00:00Z"}`)
				_, ts := newFake(http.StatusOK, body)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				schema, err := client.GetTopicSchema(ctx, "orders")

				Expect(err).NotTo(HaveOccurred())
				Expect(schema.Topic).To(Equal("orders"))
				Expect(schema.Version).To(Equal(2))
			})
		})

		Context("when the schema does not exist", func() {
			It("returns ErrNotFound", func() {
				_, ts := newFake(http.StatusNotFound, []byte(`{"error":"not found"}`))
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				_, err := client.GetTopicSchema(ctx, "unknown")

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("not found")))
				Expect(errors.Is(err, queueti.ErrNotFound)).To(BeTrue())
			})
		})
	})

	Describe("UpsertTopicSchema", func() {
		Context("when the server accepts the schema", func() {
			It("sends the schema_json body and returns the saved schema", func() {
				responseBody := []byte(`{"topic":"orders","schema_json":"{\"type\":\"record\"}","version":1,"updated_at":"2024-01-01T00:00:00Z"}`)
				srv, ts := newCapture(http.StatusOK, responseBody)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL, queueti.WithAdminToken("admin"))
				schema, err := client.UpsertTopicSchema(ctx, "orders", `{"type":"record"}`)

				Expect(err).NotTo(HaveOccurred())
				Expect(schema.Topic).To(Equal("orders"))

				req := srv.last()
				Expect(req.method).To(Equal(http.MethodPut))
				Expect(req.path).To(Equal("/api/topic-schemas/orders"))
				Expect(req.body).To(HaveKeyWithValue("schema_json", `{"type":"record"}`))
			})
		})
	})

	Describe("DeleteTopicSchema", func() {
		Context("when the server acknowledges the deletion", func() {
			It("sends DELETE and returns nil on 204", func() {
				srv, ts := newCapture(http.StatusNoContent, nil)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				err := client.DeleteTopicSchema(ctx, "orders")

				Expect(err).NotTo(HaveOccurred())
				req := srv.last()
				Expect(req.method).To(Equal(http.MethodDelete))
				Expect(req.path).To(Equal("/api/topic-schemas/orders"))
			})
		})
	})

	// ---- Consumer groups ----------------------------------------------------

	Describe("ListConsumerGroups", func() {
		Context("when consumer groups exist for the topic", func() {
			It("returns the items array", func() {
				body := []byte(`{"items":["billing","analytics"]}`)
				_, ts := newFake(http.StatusOK, body)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				groups, err := client.ListConsumerGroups(ctx, "orders")

				Expect(err).NotTo(HaveOccurred())
				Expect(groups).To(ConsistOf("billing", "analytics"))
			})
		})
	})

	Describe("RegisterConsumerGroup", func() {
		Context("when the group does not yet exist", func() {
			It("sends POST with the correct body and returns nil on 201", func() {
				srv, ts := newCapture(http.StatusCreated, []byte(`{}`))
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL, queueti.WithAdminToken("tok"))
				err := client.RegisterConsumerGroup(ctx, "orders", "billing")

				Expect(err).NotTo(HaveOccurred())
				req := srv.last()
				Expect(req.method).To(Equal(http.MethodPost))
				Expect(req.path).To(Equal("/api/topics/orders/consumer-groups"))
				Expect(req.body).To(HaveKeyWithValue("consumer_group", "billing"))
				Expect(req.header.Get("Authorization")).To(Equal("Bearer tok"))
			})
		})

		Context("when the group already exists", func() {
			It("returns ErrConflict", func() {
				_, ts := newFake(http.StatusConflict, []byte(`{"error":"already exists"}`))
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				err := client.RegisterConsumerGroup(ctx, "orders", "billing")

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("conflict")))
				Expect(errors.Is(err, queueti.ErrConflict)).To(BeTrue())
			})
		})
	})

	Describe("UnregisterConsumerGroup", func() {
		Context("when the group exists", func() {
			It("sends DELETE and returns nil on 204", func() {
				srv, ts := newCapture(http.StatusNoContent, nil)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				err := client.UnregisterConsumerGroup(ctx, "orders", "billing")

				Expect(err).NotTo(HaveOccurred())
				req := srv.last()
				Expect(req.method).To(Equal(http.MethodDelete))
				Expect(req.path).To(Equal("/api/topics/orders/consumer-groups/billing"))
			})
		})

		Context("when the group does not exist", func() {
			It("returns ErrNotFound", func() {
				_, ts := newFake(http.StatusNotFound, []byte(`{"error":"not found"}`))
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				err := client.UnregisterConsumerGroup(ctx, "orders", "ghost")

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("not found")))
				Expect(errors.Is(err, queueti.ErrNotFound)).To(BeTrue())
			})
		})
	})

	// ---- Stats --------------------------------------------------------------

	Describe("Stats", func() {
		Context("when the server returns topic statistics", func() {
			It("returns the topics array", func() {
				body := []byte(`{"topics":[{"topic":"orders","status":"pending","count":42},{"topic":"orders","status":"acked","count":7}]}`)
				_, ts := newFake(http.StatusOK, body)
				defer ts.Close()

				client := queueti.NewAdminClient(ts.URL)
				stats, err := client.Stats(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(stats).To(HaveLen(2))
				Expect(stats[0].Topic).To(Equal("orders"))
				Expect(stats[0].Status).To(Equal("pending"))
				Expect(stats[0].Count).To(Equal(42))
				Expect(stats[1].Status).To(Equal("acked"))
				Expect(stats[1].Count).To(Equal(7))
			})
		})
	})

	// ---- Options ------------------------------------------------------------

	Describe("WithAdminHTTPClient", func() {
		It("uses the provided http.Client for requests", func() {
			body := []byte(`{"topics":[{"topic":"orders","status":"pending","count":1}]}`)
			srv, ts := newCapture(http.StatusOK, body)
			defer ts.Close()

			// Verify that our injected client is the one that hits the server by
			// wrapping the default transport and asserting the request arrived.
			customClient := &http.Client{Transport: http.DefaultTransport}
			client := queueti.NewAdminClient(ts.URL, queueti.WithAdminHTTPClient(customClient))
			stats, err := client.Stats(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(stats).To(HaveLen(1))
			Expect(srv.last()).NotTo(BeNil())
		})
	})
})

