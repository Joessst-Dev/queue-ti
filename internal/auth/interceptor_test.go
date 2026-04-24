package auth_test

import (
	"context"
	"encoding/base64"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/config"
)

var _ = Describe("UnaryInterceptor", func() {
	var (
		cfg         config.AuthConfig
		handlerCalled bool
		handler     grpc.UnaryHandler
	)

	BeforeEach(func() {
		handlerCalled = false
		handler = func(ctx context.Context, req any) (any, error) {
			handlerCalled = true
			return "ok", nil
		}
	})

	basicAuthHeader := func(username, password string) string {
		encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		return "Basic " + encoded
	}

	ctxWithAuth := func(header string) context.Context {
		md := metadata.Pairs("authorization", header)
		return metadata.NewIncomingContext(context.Background(), md)
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

		It("passes through even when authorization header is present", func() {
			intercept := auth.UnaryInterceptor(cfg)
			resp, err := intercept(ctxWithAuth(basicAuthHeader("user", "pass")), nil, nil, handler)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("ok"))
		})
	})

	Context("when auth is enabled", func() {
		BeforeEach(func() {
			cfg = config.AuthConfig{
				Enabled:  true,
				Username: "admin",
				Password: "secret",
			}
		})

		Context("with valid credentials", func() {
			It("calls the handler and returns its response", func() {
				intercept := auth.UnaryInterceptor(cfg)
				resp, err := intercept(ctxWithAuth(basicAuthHeader("admin", "secret")), nil, nil, handler)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("ok"))
				Expect(handlerCalled).To(BeTrue())
			})
		})

		Context("with missing metadata", func() {
			It("returns Unauthenticated", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(context.Background(), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("missing metadata"))
			})
		})

		Context("with metadata but no authorization header", func() {
			It("returns Unauthenticated", func() {
				md := metadata.Pairs("x-other", "value")
				ctx := metadata.NewIncomingContext(context.Background(), md)
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctx, nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("missing authorization header"))
			})
		})

		Context("with an unsupported auth scheme", func() {
			It("returns Unauthenticated", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithAuth("Bearer sometoken"), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid authorization format"))
			})
		})

		Context("with malformed base64 after Basic prefix", func() {
			It("returns Unauthenticated", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithAuth("Basic !!!not-base64!!!"), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid authorization format"))
			})
		})

		Context("with base64 that decodes to a value without a colon", func() {
			It("returns Unauthenticated", func() {
				encoded := base64.StdEncoding.EncodeToString([]byte("nocolon"))
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithAuth("Basic "+encoded), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid authorization format"))
			})
		})

		Context("with wrong username", func() {
			It("returns Unauthenticated", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithAuth(basicAuthHeader("wrong", "secret")), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid credentials"))
			})
		})

		Context("with wrong password", func() {
			It("returns Unauthenticated", func() {
				intercept := auth.UnaryInterceptor(cfg)
				_, err := intercept(ctxWithAuth(basicAuthHeader("admin", "wrong")), nil, nil, handler)

				Expect(handlerCalled).To(BeFalse())
				Expect(status.Code(err)).To(Equal(codes.Unauthenticated))
				Expect(err.Error()).To(ContainSubstring("invalid credentials"))
			})
		})

		Context("with a password containing a colon", func() {
			It("handles it correctly by splitting only on the first colon", func() {
				cfg.Password = "sec:ret"
				intercept := auth.UnaryInterceptor(cfg)
				resp, err := intercept(ctxWithAuth(basicAuthHeader("admin", "sec:ret")), nil, nil, handler)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("ok"))
			})
		})
	})
})
