package queueti

import (
	"context"
	"crypto/tls"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// DialOption configures the behaviour of Dial.
type DialOption func(*dialConfig)

// TokenRefresher is a function the library calls to obtain a fresh JWT.
type TokenRefresher func(ctx context.Context) (string, error)

type dialConfig struct {
	grpcOpts  []grpc.DialOption
	store     *tokenStore // non-nil when WithBearerToken or WithTokenRefresher used
	refresher TokenRefresher
}

// WithInsecure disables transport security (suitable for local/dev use).
func WithInsecure() DialOption {
	return func(cfg *dialConfig) {
		cfg.grpcOpts = append(cfg.grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
}

// WithBearerToken attaches a JWT Bearer token to every RPC call via the
// "authorization" metadata header. The token can be updated later via
// Client.SetToken without reconnecting.
func WithBearerToken(token string) DialOption {
	return func(cfg *dialConfig) {
		if cfg.store == nil {
			cfg.store = newTokenStore(token)
		} else {
			cfg.store.set(token)
		}
		cfg.grpcOpts = append(cfg.grpcOpts, grpc.WithPerRPCCredentials(bearerToken{store: cfg.store}))
	}
}

// WithTokenRefresher registers a callback the client calls automatically to
// obtain a fresh JWT ~60 s before the current token expires.
// Use together with WithBearerToken to provide the initial token.
func WithTokenRefresher(r TokenRefresher) DialOption {
	return func(cfg *dialConfig) {
		cfg.refresher = r
	}
}

// WithTLS configures custom TLS transport credentials. Pass a *tls.Config to
// supply a custom CA pool, client certificate (mTLS), or server name override.
// Mutually exclusive with WithInsecure.
//
// Common patterns:
//
//	// Custom CA (self-signed server):
//	pool := x509.NewCertPool()
//	pool.AppendCertsFromPEM(caPEM)
//	queueti.WithTLS(&tls.Config{RootCAs: pool})
//
//	// mTLS:
//	cert, _ := tls.X509KeyPair(certPEM, keyPEM)
//	queueti.WithTLS(&tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}})
//
//	// Server name override (self-signed cert with different hostname):
//	queueti.WithTLS(&tls.Config{RootCAs: pool, ServerName: "myserver.internal"})
func WithTLS(cfg *tls.Config) DialOption {
	return func(d *dialConfig) {
		d.grpcOpts = append(d.grpcOpts, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	}
}

// WithGRPCOption passes a raw grpc.DialOption through for advanced use.
func WithGRPCOption(o grpc.DialOption) DialOption {
	return func(cfg *dialConfig) {
		cfg.grpcOpts = append(cfg.grpcOpts, o)
	}
}

// bearerToken implements credentials.PerRPCCredentials reading from a shared
// tokenStore so the token can be updated without reconnecting.
type bearerToken struct {
	store *tokenStore
}

var _ credentials.PerRPCCredentials = bearerToken{}

func (b bearerToken) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.store.get(),
	}, nil
}

func (b bearerToken) RequireTransportSecurity() bool { return false }

// ConsumerOption configures a Consumer.
type ConsumerOption func(*consumerConfig)

type consumerConfig struct {
	concurrency       int
	visibilityTimeout uint32 // seconds, 0 = server default
	consumerGroup     string
}

// WithConcurrency sets the number of concurrent handler goroutines (default 1).
func WithConcurrency(n int) ConsumerOption {
	return func(cfg *consumerConfig) {
		cfg.concurrency = n
	}
}

// WithVisibilityTimeout sets a custom visibility timeout in seconds.
func WithVisibilityTimeout(seconds uint32) ConsumerOption {
	return func(cfg *consumerConfig) {
		cfg.visibilityTimeout = seconds
	}
}

// WithConsumerGroup configures the consumer to process messages as part of a
// named group. Multiple consumers with the same group on the same topic each
// receive every message independently. An empty string (the default) uses the
// legacy single-consumer behaviour.
func WithConsumerGroup(group string) ConsumerOption {
	return func(cfg *consumerConfig) {
		cfg.consumerGroup = group
	}
}

// PublishOption configures a Publish call.
type PublishOption func(*publishConfig)

type publishConfig struct {
	metadata map[string]string
	key      *string
}

// WithMetadata attaches key-value metadata to the published message.
func WithMetadata(m map[string]string) PublishOption {
	return func(cfg *publishConfig) {
		cfg.metadata = m
	}
}

// WithKey sets a deduplication key on the published message. When a pending
// message with the same (topic, key) pair already exists it is upserted rather
// than creating a new row.
func WithKey(key string) PublishOption {
	return func(cfg *publishConfig) {
		cfg.key = &key
	}
}
