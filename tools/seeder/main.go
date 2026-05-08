package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		seedFile string
		adminURL string
		token    string
		username string
		password string
		dryRun   bool
		timeout  time.Duration
	)

	cmd := &cobra.Command{
		Use:   "seeder -f <seed-file>",
		Short: "Idempotently provision queue-ti resources from a JSON seed file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))
			hc := &http.Client{Timeout: timeout}

			seed, err := loadSeed(seedFile)
			if err != nil {
				return err
			}

			authToken, err := resolveToken(cmd.Context(), adminURL, token, username, password, hc)
			if err != nil {
				return err
			}

			opts := []queueti.AdminOption{queueti.WithAdminHTTPClient(hc)}
			if authToken != "" {
				opts = append(opts, queueti.WithAdminToken(authToken))
			}

			seeder := newSeeder(queueti.NewAdminClient(adminURL, opts...), dryRun, log)
			if err := seeder.Apply(cmd.Context(), seed); err != nil {
				return err
			}

			log.Info("seeding complete")
			return nil
		},
	}

	cmd.Flags().StringVarP(&seedFile, "file", "f", "", "path to seed JSON file (required)")
	cmd.Flags().StringVar(&adminURL, "admin-url", "http://localhost:8080", "base URL of the admin HTTP API")
	cmd.Flags().StringVar(&token, "token", "", "static bearer token for authentication")
	cmd.Flags().StringVar(&username, "username", "", "username for login-based auth")
	cmd.Flags().StringVar(&password, "password", "", "password for login-based auth (prefer SEEDER_PASSWORD env var)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print planned actions without calling the API")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "per-request HTTP timeout")

	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// loadSeed reads, parses, and validates a seed file.
func loadSeed(path string) (*SeedFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed file: %w", err)
	}
	var seed SeedFile
	if err := json.Unmarshal(raw, &seed); err != nil {
		return nil, fmt.Errorf("parse seed file: %w", err)
	}
	if err := seed.validate(); err != nil {
		return nil, fmt.Errorf("invalid seed file: %w", err)
	}
	return &seed, nil
}

// resolveToken returns a bearer token from the given sources, in priority order:
// explicit --token flag, then username+password login, then empty (no auth).
func resolveToken(ctx context.Context, adminURL, token, username, password string, hc *http.Client) (string, error) {
	if token != "" {
		return token, nil
	}
	if username == "" {
		return "", nil
	}
	// Prefer SEEDER_PASSWORD env var to avoid exposing the password in ps output.
	if password == "" {
		password = os.Getenv("SEEDER_PASSWORD")
	}
	if password == "" {
		return "", fmt.Errorf("--password or SEEDER_PASSWORD is required when --username is set")
	}
	auth, err := queueti.NewAuth(ctx, adminURL, username, password, queueti.WithAuthHTTPClient(hc))
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}
	return auth.Token(), nil
}
