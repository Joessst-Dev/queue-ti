package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/users"
	pb "github.com/Joessst-Dev/queue-ti/pb"
)

type GRPCServer struct {
	pb.UnimplementedQueueServiceServer
	queueService *queue.Service
	userStore    *users.Store // nil when auth is disabled
}

func NewGRPCServer(qs *queue.Service, us *users.Store) *GRPCServer {
	return &GRPCServer{queueService: qs, userStore: us}
}

// resolveVisibilityTimeoutFromProto converts the optional uint32 proto field
// into a time.Duration. A nil pointer means "use server default" and returns
// zero. A zero value is explicitly rejected with an error because clients that
// set the field to zero almost certainly intended a positive value.
func resolveVisibilityTimeoutFromProto(ptr *uint32) (time.Duration, error) {
	if ptr == nil {
		return 0, nil
	}
	if *ptr == 0 {
		return 0, status.Error(codes.InvalidArgument, "visibility_timeout_seconds must be greater than zero")
	}
	return time.Duration(*ptr) * time.Second, nil
}

// checkGrant verifies the caller has the required action/topic permission.
// It is a no-op when auth is disabled (userStore is nil) or when claims are
// absent (interceptor didn't run). Admins are unconditionally allowed.
func (s *GRPCServer) checkGrant(ctx context.Context, action, topic string) error {
	if s.userStore == nil {
		return nil
	}
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil // auth disabled — interceptor didn't inject claims
	}
	if claims.IsAdmin {
		return nil
	}
	grants, err := s.userStore.GetUserGrants(ctx, claims.UserID)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to check grants: %v", err)
	}
	if !users.HasGrant(grants, action, topic) {
		return status.Errorf(codes.PermissionDenied, "insufficient permissions")
	}
	return nil
}

func (s *GRPCServer) Enqueue(ctx context.Context, req *pb.EnqueueRequest) (*pb.EnqueueResponse, error) {
	if req.Topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic is required")
	}
	if len(req.Payload) == 0 {
		return nil, status.Error(codes.InvalidArgument, "payload is required")
	}

	if err := s.checkGrant(ctx, "write", req.Topic); err != nil {
		return nil, err
	}

	id, err := s.queueService.Enqueue(ctx, req.Topic, req.Payload, req.Metadata, req.Key)
	if err != nil {
		if errors.Is(err, queue.ErrSchemaValidation) {
			return nil, status.Errorf(codes.InvalidArgument, "%s", err)
		}
		if errors.Is(err, queue.ErrTopicNotRegistered) {
			return nil, status.Errorf(codes.FailedPrecondition, "%s", err)
		}
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

	vt, err := resolveVisibilityTimeoutFromProto(req.VisibilityTimeoutSeconds)
	if err != nil {
		return nil, err
	}

	if err := s.checkGrant(ctx, "write", req.Topic); err != nil {
		return nil, err
	}

	var msg *queue.Message
	if req.ConsumerGroup != "" {
		msg, err = s.queueService.DequeueForGroup(ctx, req.Topic, req.ConsumerGroup, vt)
	} else {
		msg, err = s.queueService.Dequeue(ctx, req.Topic, vt)
	}
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
		Key:        msg.Key,
	}, nil
}

func (s *GRPCServer) BatchDequeue(ctx context.Context, req *pb.BatchDequeueRequest) (*pb.BatchDequeueResponse, error) {
	if req.Topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic is required")
	}
	if req.Count < 1 || req.Count > 1000 {
		return nil, status.Error(codes.InvalidArgument, "count must be between 1 and 1000")
	}

	vt, err := resolveVisibilityTimeoutFromProto(req.VisibilityTimeoutSeconds)
	if err != nil {
		return nil, err
	}

	if err := s.checkGrant(ctx, "write", req.Topic); err != nil {
		return nil, err
	}

	var batch []*queue.Message
	if req.ConsumerGroup != "" {
		batch, err = s.queueService.DequeueNForGroup(ctx, req.Topic, req.ConsumerGroup, int(req.Count), vt)
	} else {
		batch, err = s.queueService.DequeueN(ctx, req.Topic, int(req.Count), vt)
	}
	if err != nil {
		if errors.Is(err, queue.ErrInvalidBatchSize) {
			return nil, status.Errorf(codes.InvalidArgument, "%s", err)
		}
		slog.Error("grpc batch dequeue failed", "topic", req.Topic, "count", req.Count, "error", err)
		return nil, status.Errorf(codes.Internal, "failed to batch dequeue: %v", err)
	}

	slog.Debug("grpc batch dequeue", "topic", req.Topic, "returned", len(batch))

	msgs := make([]*pb.DequeueResponse, len(batch))
	for i, msg := range batch {
		msgs[i] = &pb.DequeueResponse{
			Id:         msg.ID,
			Topic:      msg.Topic,
			Payload:    msg.Payload,
			Metadata:   msg.Metadata,
			CreatedAt:  timestamppb.New(msg.CreatedAt),
			RetryCount: int32(msg.RetryCount),
			MaxRetries: int32(msg.MaxRetries),
			Key:        msg.Key,
		}
	}

	return &pb.BatchDequeueResponse{Messages: msgs}, nil
}

func (s *GRPCServer) Ack(ctx context.Context, req *pb.AckRequest) (*pb.AckResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	topic, err := s.queueService.TopicForMessage(ctx, req.Id)
	if err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "message not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to look up message: %v", err)
	}
	if err := s.checkGrant(ctx, "write", topic); err != nil {
		return nil, err
	}

	var ackErr error
	if req.ConsumerGroup != "" {
		ackErr = s.queueService.AckForGroup(ctx, req.Id, req.ConsumerGroup)
	} else {
		ackErr = s.queueService.Ack(ctx, req.Id)
	}
	if ackErr != nil {
		if errors.Is(ackErr, queue.ErrNotFound) || errors.Is(ackErr, queue.ErrNotProcessing) {
			return nil, status.Error(codes.NotFound, ackErr.Error())
		}
		slog.Error("grpc ack failed", "id", req.Id, "error", ackErr)
		return nil, status.Errorf(codes.Internal, "failed to ack: %v", ackErr)
	}

	slog.Debug("grpc ack", "id", req.Id)
	return &pb.AckResponse{}, nil
}

func (s *GRPCServer) Subscribe(req *pb.SubscribeRequest, stream pb.QueueService_SubscribeServer) error {
	if req.Topic == "" {
		return status.Error(codes.InvalidArgument, "topic is required")
	}

	vt, err := resolveVisibilityTimeoutFromProto(req.VisibilityTimeoutSeconds)
	if err != nil {
		return err
	}

	if err := s.checkGrant(stream.Context(), "write", req.Topic); err != nil {
		return err
	}

	const (
		minBackoff = 250 * time.Millisecond
		maxBackoff = 2 * time.Second
	)
	backoff := minBackoff

	consumerGroup := req.ConsumerGroup

	for {
		if stream.Context().Err() != nil {
			return nil
		}

		var msg *queue.Message
		var err error
		if consumerGroup != "" {
			msg, err = s.queueService.DequeueForGroup(stream.Context(), req.Topic, consumerGroup, vt)
		} else {
			msg, err = s.queueService.Dequeue(stream.Context(), req.Topic, vt)
		}
		if err != nil {
			if stream.Context().Err() != nil {
				return nil
			}
			if errors.Is(err, queue.ErrNoMessage) {
				select {
				case <-time.After(backoff):
				case <-stream.Context().Done():
					return nil
				}
				if backoff < maxBackoff {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}
				continue
			}
			slog.Error("grpc subscribe dequeue failed", "topic", req.Topic, "error", err)
			return status.Errorf(codes.Internal, "failed to dequeue: %v", err)
		}

		backoff = minBackoff

		if err := stream.Send(&pb.SubscribeResponse{
			Id:        msg.ID,
			Topic:     msg.Topic,
			Payload:   msg.Payload,
			Metadata:  msg.Metadata,
			CreatedAt: timestamppb.New(msg.CreatedAt),
			RetryCount: int32(msg.RetryCount),
		}); err != nil {
			return err
		}
	}
}

func (s *GRPCServer) Nack(ctx context.Context, req *pb.NackRequest) (*pb.NackResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	topic, err := s.queueService.TopicForMessage(ctx, req.Id)
	if err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "message not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to look up message: %v", err)
	}
	if err := s.checkGrant(ctx, "write", topic); err != nil {
		return nil, err
	}

	var nackErr error
	if req.ConsumerGroup != "" {
		nackErr = s.queueService.NackForGroup(ctx, req.Id, req.ConsumerGroup, req.Error)
	} else {
		nackErr = s.queueService.Nack(ctx, req.Id, req.Error)
	}
	if nackErr != nil {
		if errors.Is(nackErr, queue.ErrNotFound) || errors.Is(nackErr, queue.ErrNotProcessing) {
			return nil, status.Errorf(codes.NotFound, "message not found or not in processing state: %v", nackErr)
		}
		slog.Error("grpc nack failed", "id", req.Id, "error", nackErr)
		return nil, status.Errorf(codes.Internal, "failed to nack: %v", nackErr)
	}

	slog.Debug("grpc nack", "id", req.Id)
	return &pb.NackResponse{}, nil
}
