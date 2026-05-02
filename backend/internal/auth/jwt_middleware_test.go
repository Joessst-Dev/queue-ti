package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

var _ = Describe("JWTMiddleware", func() {
	const secret = "test-secret"

	// helper: spin up a minimal Fiber app with the middleware protecting GET /protected
	newApp := func() *fiber.App {
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(auth.JWTMiddleware([]byte(secret)))
		app.Get("/protected", func(c *fiber.Ctx) error {
			claims := auth.ClaimsFromCtx(c)
			return c.JSON(fiber.Map{
				"user_id":  claims.UserID,
				"username": claims.Username,
				"is_admin": claims.IsAdmin,
			})
		})
		return app
	}

	doGet := func(app *fiber.App, authHeader string) *http.Response {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	mintToken := func(userID, username string, isAdmin bool) string {
		token, err := users.IssueToken([]byte(secret), userID, username, isAdmin)
		Expect(err).NotTo(HaveOccurred())
		return token
	}

	mintExpiredToken := func() string {
		claims := users.Claims{
			UserID:   "user-expired",
			Username: "expired",
			IsAdmin:  false,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "expired",
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(secret))
		Expect(err).NotTo(HaveOccurred())
		return signed
	}

	readBody := func(resp *http.Response) string {
		b, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		return string(b)
	}

	Context("with no Authorization header", func() {
		It("returns 401 with missing authorization header", func() {
			resp := doGet(newApp(), "")

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(readBody(resp)).To(ContainSubstring("missing authorization header"))
		})
	})

	Context("with an unsupported auth scheme", func() {
		It("returns 401 with unsupported auth scheme", func() {
			resp := doGet(newApp(), "Basic dXNlcjpwYXNz")

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(readBody(resp)).To(ContainSubstring("unsupported auth scheme"))
		})
	})

	Context("with a malformed token", func() {
		It("returns 401 with invalid or expired token", func() {
			resp := doGet(newApp(), "Bearer this.is.garbage")

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(readBody(resp)).To(ContainSubstring("invalid or expired token"))
		})
	})

	Context("with an expired token", func() {
		It("returns 401 with invalid or expired token", func() {
			resp := doGet(newApp(), "Bearer "+mintExpiredToken())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(readBody(resp)).To(ContainSubstring("invalid or expired token"))
		})
	})

	Context("with a token signed by a different secret", func() {
		It("returns 401 with invalid or expired token", func() {
			wrongToken, err := users.IssueToken([]byte("wrong-secret"), "u1", "alice", false)
			Expect(err).NotTo(HaveOccurred())

			resp := doGet(newApp(), "Bearer "+wrongToken)

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(readBody(resp)).To(ContainSubstring("invalid or expired token"))
		})
	})

	Context("with a valid token", func() {
		It("returns 200 and stores claims in the Fiber context", func() {
			token := mintToken("user-42", "alice", true)
			resp := doGet(newApp(), "Bearer "+token)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body := readBody(resp)
			Expect(body).To(ContainSubstring(`"user_id":"user-42"`))
			Expect(body).To(ContainSubstring(`"username":"alice"`))
			Expect(body).To(ContainSubstring(`"is_admin":true`))
		})

		It("stores non-admin claims correctly", func() {
			token := mintToken("user-99", "bob", false)
			resp := doGet(newApp(), "Bearer "+token)

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(readBody(resp)).To(ContainSubstring(`"is_admin":false`))
		})
	})
})
