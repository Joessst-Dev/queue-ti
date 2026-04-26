package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/server"
	pb "github.com/Joessst-Dev/queue-ti/pb"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DB)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	if cfg.Queue.DLQThreshold > cfg.Queue.MaxRetries {
		log.Printf("WARNING: queue.dlq_threshold (%d) is greater than queue.max_retries (%d); messages will be DLQ-promoted before exhausting all retries",
			cfg.Queue.DLQThreshold, cfg.Queue.MaxRetries)
	}

	queueService := queue.NewService(pool, cfg.Queue.VisibilityTimeout, cfg.Queue.MaxRetries, cfg.Queue.MessageTTL, cfg.Queue.DLQThreshold)
	queueService.StartExpiryReaper(ctx, time.Minute)

	var opts []grpc.ServerOption
	if cfg.Auth.Enabled {
		log.Println("Basic authentication enabled")
		opts = append(opts, grpc.UnaryInterceptor(auth.UnaryInterceptor(cfg.Auth)))
	}

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterQueueServiceServer(grpcServer, server.NewGRPCServer(queueService))

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	go func() {
		log.Printf("gRPC server listening on %s", addr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Start HTTP server for admin UI
	httpServer := server.NewHTTPServer(queueService, cfg.Auth)
	httpAddr := fmt.Sprintf(":%d", cfg.Server.HTTPPort)
	go func() {
		log.Printf("HTTP server listening on %s", httpAddr)
		if err := httpServer.App.Listen(httpAddr); err != nil {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	cancel()
	httpServer.App.Shutdown()
	grpcServer.GracefulStop()
}
