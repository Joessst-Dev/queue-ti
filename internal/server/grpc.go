package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Joessst-Dev/queue-ti/internal/queue"
	pb "github.com/Joessst-Dev/queue-ti/pb"
)

type GRPCServer struct {
	pb.UnimplementedQueueServiceServer
	queueService *queue.Service
}

func NewGRPCServer(qs *queue.Service) *GRPCServer {
	return &GRPCServer{queueService: qs}
}

func (s *GRPCServer) Enqueue(ctx context.Context, req *pb.EnqueueRequest) (*pb.EnqueueResponse, error) {
	if req.Topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic is required")
	}
	if len(req.Payload) == 0 {
		return nil, status.Error(codes.InvalidArgument, "payload is required")
	}

	id, err := s.queueService.Enqueue(ctx, req.Topic, req.Payload, req.Metadata)
	if err != nil {
		slog.Error("grpc enqueue failed", "topic", req.Topic, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to enqueue: %v", err)
	}

	slog.Debug("grpc enqueue", "topic", req.Topic, "id", id)
	return &pb.EnqueueResponse{Id: id}, nil
}

func (s *GRPCServer) Dequeue(ctx context.Context, req *pb.DequeueRequest) (*pb.DequeueResponse, error) {
	if req.Topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic is required")
	}

	var vt time.Duration
	if req.VisibilityTimeoutSeconds != nil {
		if *req.VisibilityTimeoutSeconds == 0 {
			return nil, status.Error(codes.InvalidArgument, "visibility_timeout_seconds must be greater than zero")
		}
		vt = time.Duration(*req.VisibilityTimeoutSeconds) * time.Second
	}

	msg, err := s.queueService.Dequeue(ctx, req.Topic, vt)
	if err != nil {
		if errors.Is(err, queue.ErrNoMessage) {
			slog.Debug("grpc dequeue: no messages", "topic", req.Topic)
			return nil, status.Error(codes.NotFound, "no messages available")
		}
		slog.Error("grpc dequeue failed", "topic", req.Topic, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to dequeue: %v", err)
	}
	slog.Debug("grpc dequeue", "topic", req.Topic, "id", msg.ID)

	return &pb.DequeueResponse{
		Id:         msg.ID,
		Topic:      msg.Topic,
		Payload:    msg.Payload,
		Metadata:   msg.Metadata,
		CreatedAt:  timestamppb.New(msg.CreatedAt),
		RetryCount: int32(msg.RetryCount),
		MaxRetries: int32(msg.MaxRetries),
	}, nil
}

func (s *GRPCServer) Ack(ctx context.Context, req *pb.AckRequest) (*pb.AckResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.queueService.Ack(ctx, req.Id); err != nil {
		slog.Error("grpc ack failed", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to ack: %v", err)
	}

	slog.Debug("grpc ack", "id", req.Id)
	return &pb.AckResponse{}, nil
}

func (s *GRPCServer) Nack(ctx context.Context, req *pb.NackRequest) (*pb.NackResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.queueService.Nack(ctx, req.Id, req.Error); err != nil {
		if errors.Is(err, queue.ErrNotFound) || errors.Is(err, queue.ErrNotProcessing) {
			return nil, status.Errorf(codes.NotFound, "message not found or not in processing state: %v", err)
		}
		slog.Error("grpc nack failed", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to nack: %v", err)
	}

	slog.Debug("grpc nack", "id", req.Id)
	return &pb.NackResponse{}, nil
}
