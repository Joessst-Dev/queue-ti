package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/metrics"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/server"
	"github.com/Joessst-Dev/queue-ti/internal/users"
	pb "github.com/Joessst-Dev/queue-ti/pb"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Logger not yet configured — fall back to a plain error before exit.
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		slog.Error("invalid log_level, must be debug|info|warn|error", "value", cfg.LogLevel)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	slog.Info("config loaded",
		"log_level", logLevel.String(),
		"grpc_port", cfg.Server.Port,
		"http_port", cfg.Server.HTTPPort,
		"visibility_timeout", cfg.Queue.VisibilityTimeout,
		"max_retries", cfg.Queue.MaxRetries,
		"dlq_threshold", cfg.Queue.DLQThreshold,
		"message_ttl", cfg.Queue.MessageTTL,
		"auth_enabled", cfg.Auth.Enabled,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connected")

	if err := db.Migrate(ctx, pool); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations applied")

	userStore := users.NewStore(pool)
	if cfg.Auth.Enabled {
		if cfg.Auth.JWTSecret == "" {
			slog.Error("auth.enabled is true but auth.jwt_secret is not set")
			os.Exit(1)
		}
		if err := users.SeedAdminUser(ctx, userStore, cfg.Auth.Username, cfg.Auth.Password); err != nil {
			slog.Error("failed to seed admin user", "error", err)
			os.Exit(1)
		}
	}

	if cfg.Queue.DLQThreshold > cfg.Queue.MaxRetries {
		slog.Warn("dlq_threshold exceeds max_retries: messages will be DLQ-promoted before exhausting all retries",
			"dlq_threshold", cfg.Queue.DLQThreshold,
			"max_retries", cfg.Queue.MaxRetries,
		)
	}

	reg := prometheus.NewRegistry()
	rec := metrics.New(pool, reg)

	queueService := queue.NewService(pool, cfg.Queue.VisibilityTimeout, cfg.Queue.MaxRetries, cfg.Queue.MessageTTL, cfg.Queue.DLQThreshold, rec)
	queueService.StartExpiryReaper(ctx, time.Minute)

	var opts []grpc.ServerOption
	if cfg.Auth.Enabled {
		slog.Info("JWT authentication enabled")
		opts = append(opts, grpc.UnaryInterceptor(auth.UnaryInterceptor(cfg.Auth)))
	}

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterQueueServiceServer(grpcServer, server.NewGRPCServer(queueService, userStore))

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("failed to listen", "addr", addr, "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("gRPC server listening", "addr", addr)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	httpServer := server.NewHTTPServer(queueService, cfg.Auth, reg, userStore)
	httpAddr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
	go func() {
		slog.Info("HTTP server listening", "addr", httpAddr)
		if err := httpServer.App.Listen(httpAddr); err != nil {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	cancel()
	httpServer.App.Shutdown()
	grpcServer.GracefulStop()
}
