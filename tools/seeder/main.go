package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"
)

func main() {
	var seedFilePath string
	flag.StringVar(&seedFilePath, "file", "", "path to seed JSON file (required)")
	flag.StringVar(&seedFilePath, "f", "", "shorthand for -file")

	adminURL := flag.String("admin-url", "http://localhost:8080", "base URL of the admin HTTP API")
	token := flag.String("token", "", "static bearer token for authentication")
	username := flag.String("username", "", "username for login-based auth")
	password := flag.String("password", "", "password for login-based auth (or set SEEDER_PASSWORD)")
	dryRun := flag.Bool("dry-run", false, "print planned actions without calling the API")
	timeout := flag.Duration("timeout", 30*time.Second, "per-request HTTP timeout")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if seedFilePath == "" {
		fmt.Fprintln(os.Stderr, "error: -file is required")
		flag.Usage()
		os.Exit(1)
	}

	// Prefer SEEDER_PASSWORD env var to avoid exposing the password in ps output.
	resolvedPassword := *password
	if resolvedPassword == "" {
		resolvedPassword = os.Getenv("SEEDER_PASSWORD")
	}

	raw, err := os.ReadFile(seedFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read seed file: %v\n", err)
		os.Exit(1)
	}

	var seed SeedFile
	if err := json.Unmarshal(raw, &seed); err != nil {
		fmt.Fprintf(os.Stderr, "error: parse seed file: %v\n", err)
		os.Exit(1)
	}

	if err := seed.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid seed file: %v\n", err)
		os.Exit(1)
	}

	authToken := *token
	if authToken == "" && *username != "" {
		authToken, err = login(context.Background(), *adminURL, *username, resolvedPassword, *timeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: login: %v\n", err)
			os.Exit(1)
		}
	}

	var adminOpts []queueti.AdminOption
	adminOpts = append(adminOpts, queueti.WithAdminHTTPClient(&http.Client{Timeout: *timeout}))
	if authToken != "" {
		adminOpts = append(adminOpts, queueti.WithAdminToken(authToken))
	}

	admin := queueti.NewAdminClient(*adminURL, adminOpts...)
	seeder := newSeeder(admin, *dryRun, log)

	if err := seeder.Apply(context.Background(), &seed); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	log.Info("seeding complete")
}

func login(ctx context.Context, adminURL, username, password string, timeout time.Duration) (string, error) {
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		return "", fmt.Errorf("marshal login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, adminURL+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: timeout}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return result.Token, nil
}
