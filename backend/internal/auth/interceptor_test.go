package auth_test

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

var _ = Describe("UnaryInterceptor", func() {
	var (
		cfg           config.AuthConfig
		handlerCalled bool
		handler       grpc.UnaryHandler
	)

	BeforeEach(func() {
		handlerCalled = false
		handler = func(ctx context.Context, req any) (any, error) {
			handlerCalled = true
			return "ok", nil
		}
	})

	ctxWithBearerToken := func(token string) context.Context {
		md := metadata.Pairs("authorization", "Bearer "+token)
		return metadata.NewIncomingContext(context.Background(), md)
	}

	ctxWithAuthHeader := func(header string) context.Context {
		md := metadata.Pairs("authorization", header)
		return metadata.NewIncomingContext(context.Background(), md)
	}

	mintValidToken := func(secret []byte) string {
		token, err := users.IssueToken(secret, "user-123", "testuser", false)
		Expect(err).NotTo(HaveOccurred())
		return token
	}

	mintExpiredToken := func(secret []byte) string {
		claims := users.Claims{
			UserID:   "user-456",
			Username: "expireduser",
			IsAdmin:  false,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "expireduser",
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString(secret)
		Expect(err).NotTo(HaveOccurred())
		return signed
	}

	Context("when auth is disabled", func() {
		BeforeEach(func() {
			cfg = config.AuthConfig{Enabled: false}
		})

		It("passes every request through to the handler", func() {
			intercept := auth.UnaryInterceptor(cfg)
			resp, err := intercept(context.Background(), nil, nil, handler)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("ok"))
			Expect(handlerCalled).To(BeTrue())
		})

		It("passes through even when an authorization header is present", func() {
			intercept := auth.UnaryInterceptor(cfg)
			resp, err := intercept(ctxWithBearerToken("sometoken"), nil, nil, handler)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("ok"))
			Expect(handlerCalled).To(BeTrue())
		})
	})

	Context("when auth is enabled", func() {
		BeforeEach(func() {
			cfg = config.AuthConfig{
				Enabled:   true,
				JWTSecret: "test-secret",
			}
		})

		Context("with a valid Bearer token", func() {
			It("calls the handler, returns its result, and stores claims in context", func() {
				secret := []byte(cfg.JWTSecret)
				tokenStr, err := users.IssueToken(secret, "user-123", "testuser", true)
				Expect(err).NotTo(HaveOccurred())

				var capturedCtx context.Context
				handler = func(ctx context.Context, req any) (any, error) {
					capturedCtx = ctx
					handlerCalled = true
					return "ok", nil
				}

				intercept := auth.UnaryInterceptor(cfg)
				resp, err := intercept(ctxWithBearerToken(tokenStr), nil, nil, handler)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("ok"))
				Expect(handlerCalled).To(BeTrue())

				claims := auth.ClaimsFromContext(capturedCtx)
				Expect(claims).NotTo(BeNil())
				Expect(claims.UserID).To(Equal("user-123"))
				Expect(claims.Username).To(Equal("testuser"))
				Expect(claims.IsAdmin).To(BeTrue())
			})
		})

		Context("with missing metadata", func() {
			It("returns Unauthenticated \"missing metadata\"", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(context.Background(), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("missing metadata"))
			})
		})

		Context("with metadata but no authorization header", func() {
			It("returns Unauthenticated \"missing authorization header\"", func() {
				md := metadata.Pairs("x-other", "value")
				ctx := metadata.NewIncomingContext(context.Background(), md)
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctx, nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("missing authorization header"))
			})
		})

		Context("with an unsupported auth scheme (Basic ...)", func() {
			It("returns Unauthenticated \"unsupported auth scheme\"", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithAuthHeader("Basic dXNlcjpwYXNz"), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("unsupported auth scheme"))
			})
		})

		Context("with a malformed/garbage token string", func() {
			It("returns Unauthenticated \"invalid or expired token\"", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithBearerToken("this.is.garbage"), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid or expired token"))
			})
		})

		Context("with an expired token", func() {
			It("returns Unauthenticated \"invalid or expired token\"", func() {
				expiredToken := mintExpiredToken([]byte(cfg.JWTSecret))
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithBearerToken(expiredToken), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid or expired token"))
			})
		})

		Context("with a token signed by a different secret", func() {
			It("returns Unauthenticated \"invalid or expired token\"", func() {
				wrongSecretToken := mintValidToken([]byte("completely-different-secret"))
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithBearerToken(wrongSecretToken), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid or expired token"))
			})
		})
	})
})
