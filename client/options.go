package queueti

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type dialOption func(*dialConfig)

type dialConfig struct {
	grpcOpts []grpc.DialOption
	token    string
}

// WithInsecure disables transport security (suitable for local/dev use).
func WithInsecure() dialOption {
	return func(cfg *dialConfig) {
		cfg.grpcOpts = append(cfg.grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
}

// WithBearerToken attaches a JWT Bearer token to every RPC call via the
// "authorization" metadata header.
func WithBearerToken(token string) dialOption {
	return func(cfg *dialConfig) {
		cfg.token = token
		cfg.grpcOpts = append(cfg.grpcOpts, grpc.WithPerRPCCredentials(bearerToken{token: token}))
	}
}

// WithGRPCOption passes a raw grpc.DialOption through for advanced use.
func WithGRPCOption(o grpc.DialOption) dialOption {
	return func(cfg *dialConfig) {
		cfg.grpcOpts = append(cfg.grpcOpts, o)
	}
}

// bearerToken implements credentials.PerRPCCredentials so that every gRPC call
// carries an "authorization: Bearer <token>" metadata header.
type bearerToken struct {
	token string
}

var _ credentials.PerRPCCredentials = bearerToken{}

func (b bearerToken) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.token,
	}, nil
}

func (b bearerToken) RequireTransportSecurity() bool {
	return false
}
