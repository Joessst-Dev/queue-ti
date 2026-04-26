package users_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Joessst-Dev/queue-ti/internal/users"
)

var _ = Describe("IssueToken / ParseToken", func() {
	var (
		secret   = []byte("test-secret-key")
		userID   = "user-uuid-1234"
		username = "alice"
		isAdmin  = true
	)

	Describe("IssueToken", func() {
		Context("with valid inputs", func() {
			It("should return a non-empty token string without error", func() {
				token, err := users.IssueToken(secret, userID, username, isAdmin)
				Expect(err).NotTo(HaveOccurred())
				Expect(token).NotTo(BeEmpty())
			})
		})
	})

	Describe("ParseToken", func() {
		Context("with a freshly issued valid token", func() {
			It("should round-trip the UserID, Username, and IsAdmin fields correctly", func() {
				token, err := users.IssueToken(secret, userID, username, isAdmin)
				Expect(err).NotTo(HaveOccurred())

				claims, err := users.ParseToken(secret, token)
				Expect(err).NotTo(HaveOccurred())
				Expect(claims.UserID).To(Equal(userID))
				Expect(claims.Username).To(Equal(username))
				Expect(claims.IsAdmin).To(Equal(isAdmin))
			})

			It("should correctly preserve IsAdmin=false", func() {
				token, err := users.IssueToken(secret, userID, "bob", false)
				Expect(err).NotTo(HaveOccurred())

				claims, err := users.ParseToken(secret, token)
				Expect(err).NotTo(HaveOccurred())
				Expect(claims.IsAdmin).To(BeFalse())
			})
		})

		Context("with an expired token", func() {
			It("should return ErrTokenInvalid", func() {
				// Craft a token with expiry in the past.
				now := time.Now()
				expiredClaims := users.Claims{
					UserID:   userID,
					Username: username,
					IsAdmin:  isAdmin,
					RegisteredClaims: jwt.RegisteredClaims{
						Subject:   username,
						IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
						ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
					},
				}
				rawToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
				tokenString, err := rawToken.SignedString(secret)
				Expect(err).NotTo(HaveOccurred())

				_, parseErr := users.ParseToken(secret, tokenString)
				Expect(parseErr).To(MatchError(users.ErrTokenInvalid))
			})
		})

		Context("with a token signed by a different secret", func() {
			It("should return ErrTokenInvalid", func() {
				token, err := users.IssueToken([]byte("other-secret"), userID, username, isAdmin)
				Expect(err).NotTo(HaveOccurred())

				_, parseErr := users.ParseToken(secret, token)
				Expect(parseErr).To(MatchError(users.ErrTokenInvalid))
			})
		})

		Context("with a tampered token (flipped byte in the signature)", func() {
			It("should return ErrTokenInvalid", func() {
				token, err := users.IssueToken(secret, userID, username, isAdmin)
				Expect(err).NotTo(HaveOccurred())

				// A JWT has three base64url segments separated by '.'.
				// Flip the last byte of the signature segment.
				parts := strings.Split(token, ".")
				Expect(parts).To(HaveLen(3))
				sig := []byte(parts[2])
				sig[len(sig)-1] ^= 0xFF
				tampered := parts[0] + "." + parts[1] + "." + string(sig)

				_, parseErr := users.ParseToken(secret, tampered)
				Expect(parseErr).To(MatchError(users.ErrTokenInvalid))
			})
		})

		Context("with a completely garbage string", func() {
			It("should return ErrTokenInvalid", func() {
				_, parseErr := users.ParseToken(secret, "not.a.token")
				Expect(parseErr).To(MatchError(users.ErrTokenInvalid))
			})
		})
	})
})
