package queueti_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Auth", func() {
	Describe("NewAuth", func() {
		Context("when the server does not require auth", func() {
			It("returns an Auth with an empty token without calling login", func() {
				loginCalled := false
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/auth/status":
						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(map[string]any{"auth_required": false})
					case "/api/auth/login":
						loginCalled = true
						w.WriteHeader(http.StatusInternalServerError)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer srv.Close()

				auth, err := queueti.NewAuth(context.Background(), srv.URL, "user", "pass")
				Expect(err).NotTo(HaveOccurred())
				Expect(auth.Token()).To(BeEmpty())
				Expect(loginCalled).To(BeFalse())
			})

			It("strips a trailing slash from the admin address", func() {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/auth/status" {
						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(map[string]any{"auth_required": false})
					}
				}))
				defer srv.Close()

				_, err := queueti.NewAuth(context.Background(), srv.URL+"/", "user", "pass")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when the server requires auth", func() {
			It("returns an Auth with the token from the login response", func() {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/api/auth/status":
						json.NewEncoder(w).Encode(map[string]any{"auth_required": true})
					case "/api/auth/login":
						var creds map[string]string
						json.NewDecoder(r.Body).Decode(&creds)
						Expect(creds["username"]).To(Equal("admin"))
						Expect(creds["password"]).To(Equal("secret"))
						json.NewEncoder(w).Encode(map[string]string{"token": "jwt-token-abc"})
					}
				}))
				defer srv.Close()

				auth, err := queueti.NewAuth(context.Background(), srv.URL, "admin", "secret")
				Expect(err).NotTo(HaveOccurred())
				Expect(auth.Token()).To(Equal("jwt-token-abc"))
			})

			It("encodes credentials with JSON so special characters are safe", func() {
				var capturedBody []byte
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/api/auth/status":
						json.NewEncoder(w).Encode(map[string]any{"auth_required": true})
					case "/api/auth/login":
						var body map[string]string
						json.NewDecoder(r.Body).Decode(&body)
						capturedBody, _ = json.Marshal(body)
						json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
					}
				}))
				defer srv.Close()

				_, err := queueti.NewAuth(context.Background(), srv.URL, `user"name`, `p\a"ss`)
				Expect(err).NotTo(HaveOccurred())
				var body map[string]string
				json.Unmarshal(capturedBody, &body)
				Expect(body["username"]).To(Equal(`user"name`))
				Expect(body["password"]).To(Equal(`p\a"ss`))
			})

			It("returns an error when login fails", func() {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/auth/status":
						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(map[string]any{"auth_required": true})
					case "/api/auth/login":
						w.WriteHeader(http.StatusUnauthorized)
						w.Write([]byte(`{"error":"invalid credentials"}`))
					}
				}))
				defer srv.Close()

				_, err := queueti.NewAuth(context.Background(), srv.URL, "bad", "creds")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("login"))
			})
		})

		Context("when the status endpoint is unreachable", func() {
			It("returns an error", func() {
				_, err := queueti.NewAuth(context.Background(), "http://127.0.0.1:1", "u", "p")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("check auth status"))
			})
		})
	})

	Describe("Refresh context propagation", func() {
		It("respects a cancelled context and does not perform the HTTP call", func() {
			loginCalled := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/auth/status":
					json.NewEncoder(w).Encode(map[string]any{"auth_required": true})
				case "/api/auth/login":
					loginCalled = true
					json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
				}
			}))
			defer srv.Close()

			auth, err := queueti.NewAuth(context.Background(), srv.URL, "admin", "secret")
			Expect(err).NotTo(HaveOccurred())
			loginCalled = false // reset after initial login

			ctx, cancel := context.WithCancel(context.Background())
			cancel() // cancel before calling Refresh

			_, err = auth.Refresh(ctx)
			Expect(err).To(HaveOccurred())
			Expect(loginCalled).To(BeFalse())
		})
	})

	Describe("Refresh", func() {
		Context("when auth is disabled (empty token)", func() {
			It("is a no-op and returns an empty string", func() {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"auth_required": false})
				}))
				defer srv.Close()

				auth, err := queueti.NewAuth(context.Background(), srv.URL, "u", "p")
				Expect(err).NotTo(HaveOccurred())

				tok, err := auth.Refresh(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(tok).To(BeEmpty())
			})
		})

		Context("when auth is enabled", func() {
			It("re-authenticates and returns the new token", func() {
				callCount := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.URL.Path {
					case "/api/auth/status":
						json.NewEncoder(w).Encode(map[string]any{"auth_required": true})
					case "/api/auth/login":
						callCount++
						json.NewEncoder(w).Encode(map[string]string{"token": "refreshed-token"})
					}
				}))
				defer srv.Close()

				auth, err := queueti.NewAuth(context.Background(), srv.URL, "admin", "secret")
				Expect(err).NotTo(HaveOccurred())
				Expect(callCount).To(Equal(1))

				tok, err := auth.Refresh(context.Background())
				Expect(err).NotTo(HaveOccurred())
				Expect(tok).To(Equal("refreshed-token"))
				Expect(callCount).To(Equal(2))
			})
		})
	})
})
