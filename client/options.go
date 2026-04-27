package queueti

import (
	"context"

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
