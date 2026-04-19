package server

import (
	"context"

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
		return nil, status.Errorf(codes.Internal, "failed to enqueue: %v", err)
	}

	return &pb.EnqueueResponse{Id: id}, nil
}

func (s *GRPCServer) Dequeue(ctx context.Context, req *pb.DequeueRequest) (*pb.DequeueResponse, error) {
	if req.Topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic is required")
	}

	msg, err := s.queueService.Dequeue(ctx, req.Topic)
	if err != nil {
		if err == queue.ErrNoMessage {
			return nil, status.Error(codes.NotFound, "no messages available")
		}
		return nil, status.Errorf(codes.Internal, "failed to dequeue: %v", err)
	}

	return &pb.DequeueResponse{
		Id:        msg.ID,
		Topic:     msg.Topic,
		Payload:   msg.Payload,
		Metadata:  msg.Metadata,
		CreatedAt: timestamppb.New(msg.CreatedAt),
	}, nil
}

func (s *GRPCServer) Ack(ctx context.Context, req *pb.AckRequest) (*pb.AckResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.queueService.Ack(ctx, req.Id); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to ack: %v", err)
	}

	return &pb.AckResponse{}, nil
}

