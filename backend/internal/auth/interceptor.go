package auth

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

// claimsKey is an unexported type used as the context key for JWT claims,
// preventing collisions with keys from other packages.
type claimsKey struct{}

// ClaimsFromContext retrieves the JWT claims stored by the interceptor.
// Returns nil when auth is disabled or the interceptor has not run.
func ClaimsFromContext(ctx context.Context) *users.Claims {
	v := ctx.Value(claimsKey{})
	if v == nil {
		return nil
	}
	claims, _ := v.(*users.Claims)
	return claims
}

// UnaryInterceptor returns a gRPC unary interceptor that validates
// JWT Bearer tokens from the "authorization" metadata header.
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

		if !strings.HasPrefix(authHeader[0], "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "unsupported auth scheme")
		}

		tokenStr := strings.TrimPrefix(authHeader[0], "Bearer ")
		claims, err := users.ParseToken([]byte(cfg.JWTSecret), tokenStr)
		if err != nil {
			slog.Warn("grpc jwt auth: invalid or expired token", "error", err)
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}

		enrichedCtx := context.WithValue(ctx, claimsKey{}, claims)
		return handler(enrichedCtx, req)
	}
}
