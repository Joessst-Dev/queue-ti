package auth

import (
	"context"
	"encoding/base64"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Joessst-Dev/queue-ti/internal/config"
)

// UnaryInterceptor returns a gRPC unary interceptor that validates
// basic authentication credentials from the "authorization" metadata header.
// If auth is disabled in config, all requests are allowed through.
func UnaryInterceptor(cfg config.AuthConfig) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if !cfg.Enabled {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		username, password, err := parseBasicAuth(authHeader[0])
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
		}

		if username != cfg.Username || password != cfg.Password {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		}

		return handler(ctx, req)
	}
}

// parseBasicAuth extracts username and password from a "Basic <base64>" header value.
func parseBasicAuth(header string) (string, string, error) {
	if !strings.HasPrefix(header, "Basic ") {
		return "", "", status.Error(codes.Unauthenticated, "unsupported auth scheme")
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Basic "))
	if err != nil {
		return "", "", err
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", status.Error(codes.Unauthenticated, "invalid credentials format")
	}

	return parts[0], parts[1], nil
}
