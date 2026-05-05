package queueti

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Auth encapsulates the queue-ti authentication flow against the admin HTTP
// API. It checks whether auth is required, performs the login, and implements
// TokenRefresher so it can be wired directly into Dial options.
//
// When auth is disabled on the server, NewAuth returns an Auth whose Token()
// is empty and whose Refresh method is a no-op.
type Auth struct {
	adminAddr string
	username  string
	password  string
	httpClient *http.Client

	mu    sync.RWMutex
	token string // empty when auth is disabled
}

// NewAuth connects to adminAddr, checks whether auth is required, and — when
// it is — performs a login with username/password to obtain a JWT.
//
// adminAddr should be the scheme+host+port of the admin API, e.g.
// "http://localhost:8080". A trailing slash is stripped automatically.
func NewAuth(adminAddr, username, password string) (*Auth, error) {
	a := &Auth{
		adminAddr:  strings.TrimRight(adminAddr, "/"),
		username:   username,
		password:   password,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	required, err := a.authRequired()
	if err != nil {
		return nil, fmt.Errorf("queue-ti auth: check auth status: %w", err)
	}
	if !required {
		return a, nil
	}

	if err := a.login(); err != nil {
		return nil, fmt.Errorf("queue-ti auth: login: %w", err)
	}
	return a, nil
}

// Token returns the current JWT, or an empty string when authentication is
// disabled on the server.
func (a *Auth) Token() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.token
}

// Refresh implements TokenRefresher. It re-authenticates with the server and
// updates the stored token. When auth is disabled (Token() is empty) it is a
// no-op and returns ("", nil).
func (a *Auth) Refresh(_ context.Context) (string, error) {
	if a.Token() == "" {
		return "", nil
	}
	if err := a.login(); err != nil {
		return "", fmt.Errorf("queue-ti auth: refresh: %w", err)
	}
	return a.Token(), nil
}

// authRequired calls GET /api/auth/status and returns true when the server
// requires authentication.
func (a *Auth) authRequired() (bool, error) {
	req, err := http.NewRequest(http.MethodGet, a.adminAddr+"/api/auth/status", nil)
	if err != nil {
		return false, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(raw))
	}

	var body struct {
		AuthRequired bool `json:"auth_required"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}
	return body.AuthRequired, nil
}

// login calls POST /api/auth/login and stores the returned JWT.
func (a *Auth) login() error {
	payload, err := json.Marshal(map[string]string{
		"username": a.username,
		"password": a.password,
	})
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, a.adminAddr+"/api/auth/login", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(raw))
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if body.Token == "" {
		return fmt.Errorf("server returned empty token")
	}

	a.mu.Lock()
	a.token = body.Token
	a.mu.Unlock()
	return nil
}
