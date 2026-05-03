package queueti_test

import (
	"context"
	"net"
	"sync"

	pb "github.com/Joessst-Dev/queue-ti/pb"
	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// producerFakeServer is a minimal QueueServiceServer that handles only Enqueue.
type producerFakeServer struct {
	pb.UnimplementedQueueServiceServer

	mu       sync.Mutex
	calls    []*pb.EnqueueRequest
	returnID string
	returnErr error
}

func (s *producerFakeServer) Enqueue(_ context.Context, req *pb.EnqueueRequest) (*pb.EnqueueResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	if s.returnErr != nil {
		return nil, s.returnErr
	}
	return &pb.EnqueueResponse{Id: s.returnID}, nil
}

func (s *producerFakeServer) recorded() []*pb.EnqueueRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*pb.EnqueueRequest, len(s.calls))
	copy(out, s.calls)
	return out
}

// startProducerServer starts a bufconn gRPC server backed by fake and returns
// a connected queueti.Client and a teardown function.
func startProducerServer(fake *producerFakeServer) (*queueti.Client, func()) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	srv := grpc.NewServer()
	pb.RegisterQueueServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	Expect(err).NotTo(HaveOccurred())

	client, err := queueti.DialConn(conn)
	Expect(err).NotTo(HaveOccurred())

	teardown := func() {
		_ = client.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return client, teardown
}

var _ = Describe("Producer", func() {
	var (
		fake     *producerFakeServer
		producer *queueti.Producer
		teardown func()
	)

	BeforeEach(func() {
		fake = &producerFakeServer{returnID: "msg-abc-123"}
		var client *queueti.Client
		client, teardown = startProducerServer(fake)
		producer = client.NewProducer()
	})

	AfterEach(func() {
		teardown()
	})

	Describe("Publish", func() {
		Context("when topic, payload and metadata are provided", func() {
			It("sends the correct topic, payload and metadata to the server and returns the assigned ID", func() {
				ctx := context.Background()
				id, err := producer.Publish(ctx, "orders", []byte("hello"), queueti.WithMetadata(map[string]string{
					"source": "checkout",
				}))

				Expect(err).NotTo(HaveOccurred())
				Expect(id).To(Equal("msg-abc-123"))

				calls := fake.recorded()
				Expect(calls).To(HaveLen(1))
				Expect(calls[0].Topic).To(Equal("orders"))
				Expect(calls[0].Payload).To(Equal([]byte("hello")))
				Expect(calls[0].Metadata).To(HaveKeyWithValue("source", "checkout"))
			})
		})

		Context("when no metadata option is supplied", func() {
			It("sends a nil metadata map and still returns the assigned ID", func() {
				ctx := context.Background()
				id, err := producer.Publish(ctx, "events", []byte("data"))

				Expect(err).NotTo(HaveOccurred())
				Expect(id).To(Equal("msg-abc-123"))

				calls := fake.recorded()
				Expect(calls).To(HaveLen(1))
				Expect(calls[0].Metadata).To(BeNil())
			})
		})

		Context("when the server returns an error", func() {
			BeforeEach(func() {
				fake.returnErr = status.Error(codes.Internal, "storage unavailable")
			})

			It("propagates the error wrapped with the topic name", func() {
				ctx := context.Background()
				_, err := producer.Publish(ctx, "payments", []byte("tx"))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("payments"))
				Expect(err.Error()).To(ContainSubstring("storage unavailable"))
			})
		})
	})
})
