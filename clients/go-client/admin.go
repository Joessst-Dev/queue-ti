package queueti

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrNotFound is returned when the server responds with HTTP 404.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when the server responds with HTTP 409.
var ErrConflict = errors.New("conflict")

// TopicConfig holds the optional per-topic configuration that controls
// delivery behaviour.
type TopicConfig struct {
	Topic               string `json:"topic"`
	MaxRetries          *int   `json:"max_retries,omitempty"`
	MessageTTLSeconds   *int   `json:"message_ttl_seconds,omitempty"`
	MaxDepth            *int   `json:"max_depth,omitempty"`
	Replayable          bool   `json:"replayable"`
	ReplayWindowSeconds *int   `json:"replay_window_seconds,omitempty"`
	ThroughputLimit     *int   `json:"throughput_limit,omitempty"`
}

// TopicSchema describes the Avro schema registered for a topic.
type TopicSchema struct {
	Topic      string `json:"topic"`
	SchemaJSON string `json:"schema_json"`
	Version    int    `json:"version"`
	UpdatedAt  string `json:"updated_at"`
}

// TopicStat is a single row from the stats endpoint — one entry per
// (topic, status) combination.
type TopicStat struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// AdminClient calls the queue-ti HTTP admin API (port 8080).
type AdminClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// AdminOption configures an AdminClient.
type AdminOption func(*AdminClient)

// WithAdminToken sets the Bearer token sent in every request's
// Authorization header. When empty, the header is omitted.
func WithAdminToken(token string) AdminOption {
	return func(c *AdminClient) {
		c.token = token
	}
}

// WithAdminHTTPClient replaces the default http.Client used for requests.
func WithAdminHTTPClient(hc *http.Client) AdminOption {
	return func(c *AdminClient) {
		c.httpClient = hc
	}
}

// NewAdminClient creates an AdminClient targeting baseURL.
// baseURL should be the scheme+host+port only, e.g. "http://localhost:8080".
func NewAdminClient(baseURL string, opts ...AdminOption) *AdminClient {
	c := &AdminClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ---- Topic config ----------------------------------------------------------

// ListTopicConfigs returns all topic configurations known to the server.
func (c *AdminClient) ListTopicConfigs(ctx context.Context) ([]TopicConfig, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/api/topic-configs", nil)
	if err != nil {
		return nil, fmt.Errorf("list topic configs: %w", err)
	}
	defer resp.Body.Close()

	var envelope struct {
		Items []TopicConfig `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("list topic configs: decode response: %w", err)
	}
	return envelope.Items, nil
}

// UpsertTopicConfig creates or replaces the configuration for topic.
func (c *AdminClient) UpsertTopicConfig(ctx context.Context, topic string, cfg TopicConfig) (*TopicConfig, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/api/topic-configs/"+topic, cfg)
	if err != nil {
		return nil, fmt.Errorf("upsert topic config %q: %w", topic, err)
	}
	defer resp.Body.Close()

	var out TopicConfig
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("upsert topic config %q: decode response: %w", topic, err)
	}
	return &out, nil
}

// DeleteTopicConfig removes the configuration for topic.
// Returns nil on HTTP 204.
func (c *AdminClient) DeleteTopicConfig(ctx context.Context, topic string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/api/topic-configs/"+topic, nil)
	if err != nil {
		return fmt.Errorf("delete topic config %q: %w", topic, err)
	}
	resp.Body.Close()
	return nil
}

// ---- Topic schema ----------------------------------------------------------

// ListTopicSchemas returns all topic schemas registered on the server.
func (c *AdminClient) ListTopicSchemas(ctx context.Context) ([]TopicSchema, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/api/topic-schemas", nil)
	if err != nil {
		return nil, fmt.Errorf("list topic schemas: %w", err)
	}
	defer resp.Body.Close()

	var envelope struct {
		Items []TopicSchema `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("list topic schemas: decode response: %w", err)
	}
	return envelope.Items, nil
}

// GetTopicSchema returns the registered schema for topic.
// Returns ErrNotFound when no schema exists.
func (c *AdminClient) GetTopicSchema(ctx context.Context, topic string) (*TopicSchema, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/api/topic-schemas/"+topic, nil)
	if err != nil {
		return nil, fmt.Errorf("get topic schema %q: %w", topic, err)
	}
	defer resp.Body.Close()

	var out TopicSchema
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("get topic schema %q: decode response: %w", topic, err)
	}
	return &out, nil
}

// UpsertTopicSchema creates or replaces the Avro schema for topic.
func (c *AdminClient) UpsertTopicSchema(ctx context.Context, topic, schemaJSON string) (*TopicSchema, error) {
	body := map[string]string{"schema_json": schemaJSON}
	resp, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/api/topic-schemas/"+topic, body)
	if err != nil {
		return nil, fmt.Errorf("upsert topic schema %q: %w", topic, err)
	}
	defer resp.Body.Close()

	var out TopicSchema
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("upsert topic schema %q: decode response: %w", topic, err)
	}
	return &out, nil
}

// DeleteTopicSchema removes the schema for topic.
// Returns nil on HTTP 204.
func (c *AdminClient) DeleteTopicSchema(ctx context.Context, topic string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/api/topic-schemas/"+topic, nil)
	if err != nil {
		return fmt.Errorf("delete topic schema %q: %w", topic, err)
	}
	resp.Body.Close()
	return nil
}

// ---- Consumer groups -------------------------------------------------------

// ListConsumerGroups returns the names of all consumer groups registered on topic.
func (c *AdminClient) ListConsumerGroups(ctx context.Context, topic string) ([]string, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/api/topics/"+topic+"/consumer-groups", nil)
	if err != nil {
		return nil, fmt.Errorf("list consumer groups for topic %q: %w", topic, err)
	}
	defer resp.Body.Close()

	var envelope struct {
		Items []string `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("list consumer groups for topic %q: decode response: %w", topic, err)
	}
	return envelope.Items, nil
}

// RegisterConsumerGroup creates a new consumer group on topic.
// Returns ErrConflict when the group already exists.
func (c *AdminClient) RegisterConsumerGroup(ctx context.Context, topic, group string) error {
	body := map[string]string{"consumer_group": group}
	resp, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/api/topics/"+topic+"/consumer-groups", body)
	if err != nil {
		return fmt.Errorf("register consumer group %q on topic %q: %w", group, topic, err)
	}
	resp.Body.Close()
	return nil
}

// UnregisterConsumerGroup removes group from topic.
// Returns ErrNotFound when the group does not exist.
func (c *AdminClient) UnregisterConsumerGroup(ctx context.Context, topic, group string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, c.baseURL+"/api/topics/"+topic+"/consumer-groups/"+group, nil)
	if err != nil {
		return fmt.Errorf("unregister consumer group %q on topic %q: %w", group, topic, err)
	}
	resp.Body.Close()
	return nil
}

// ---- Stats -----------------------------------------------------------------

// Stats returns per-topic message counts broken down by status.
func (c *AdminClient) Stats(ctx context.Context) ([]TopicStat, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/api/stats", nil)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	defer resp.Body.Close()

	var envelope struct {
		Topics []TopicStat `json:"topics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("stats: decode response: %w", err)
	}
	return envelope.Topics, nil
}

// ---- Internal --------------------------------------------------------------

// doRequest executes a single HTTP request. body is JSON-encoded when non-nil.
// For non-2xx responses it reads the body and returns a descriptive error,
// mapping 404 → ErrNotFound and 409 → ErrConflict.
func (c *AdminClient) doRequest(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}

	// Non-2xx — read the body for the error message then close.
	rawBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s %s: %s", ErrNotFound, method, url, string(rawBody))
	case http.StatusConflict:
		return nil, fmt.Errorf("%w: %s %s: %s", ErrConflict, method, url, string(rawBody))
	default:
		return nil, fmt.Errorf("unexpected status %d from %s %s: %s", resp.StatusCode, method, url, string(rawBody))
	}
}
