package queueti_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	pb "github.com/Joessst-Dev/queue-ti/pb"
	queueti "github.com/Joessst-Dev/queue-ti/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Fake server
// ---------------------------------------------------------------------------

// consumerFakeServer handles Subscribe, Ack, and Nack.
type consumerFakeServer struct {
	pb.UnimplementedQueueServiceServer

	mu sync.Mutex

	// messages to stream (set before starting Consume).
	messages []*pb.SubscribeResponse

	ackIDs  []string
	nackIDs []string

	// streamReady is closed once at least one Subscribe call is active.
	streamReady chan struct{}
	streamOnce  sync.Once
}

func newConsumerFakeServer(msgs []*pb.SubscribeResponse) *consumerFakeServer {
	return &consumerFakeServer{
		messages:    msgs,
		streamReady: make(chan struct{}),
	}
}

func (s *consumerFakeServer) Subscribe(req *pb.SubscribeRequest, stream grpc.ServerStreamingServer[pb.SubscribeResponse]) error {
	s.streamOnce.Do(func() { close(s.streamReady) })

	for _, msg := range s.messages {
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
	// Block until the client context is cancelled.
	<-stream.Context().Done()
	return nil
}

func (s *consumerFakeServer) Ack(_ context.Context, req *pb.AckRequest) (*pb.AckResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ackIDs = append(s.ackIDs, req.Id)
	return &pb.AckResponse{}, nil
}

func (s *consumerFakeServer) Nack(_ context.Context, req *pb.NackRequest) (*pb.NackResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nackIDs = append(s.nackIDs, req.Id)
	return &pb.NackResponse{}, nil
}

func (s *consumerFakeServer) ackedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.ackIDs))
	copy(out, s.ackIDs)
	return out
}

func (s *consumerFakeServer) nackedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.nackIDs))
	copy(out, s.nackIDs)
	return out
}

// ---------------------------------------------------------------------------
// Test helper
// ---------------------------------------------------------------------------

// startConsumerServer starts a bufconn gRPC server backed by fake and
// returns a connected Consumer and a teardown function.
func startConsumerServer(fake *consumerFakeServer, opts ...queueti.ConsumerOption) (*queueti.Consumer, func()) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	srv := grpc.NewServer()
	pb.RegisterQueueServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()

	dialCtx := context.Background()
	conn, err := grpc.DialContext( //nolint:staticcheck
		dialCtx,
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	Expect(err).NotTo(HaveOccurred())

	client, err := queueti.DialConn(conn)
	Expect(err).NotTo(HaveOccurred())

	consumer := client.NewConsumer("test-topic", opts...)

	teardown := func() {
		_ = client.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return consumer, teardown
}

// cannedMsg builds a minimal SubscribeResponse for testing.
func cannedMsg(id, topic string, payload []byte) *pb.SubscribeResponse {
	return &pb.SubscribeResponse{
		Id:        id,
		Topic:     topic,
		Payload:   payload,
		Metadata:  map[string]string{"k": "v"},
		CreatedAt: timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
		RetryCount: 2,
	}
}

// ---------------------------------------------------------------------------
// Specs
// ---------------------------------------------------------------------------

var _ = Describe("Consumer", func() {
	Describe("Consume", func() {
		Context("when the server streams two messages", func() {
			var (
				fake     *consumerFakeServer
				consumer *queueti.Consumer
				teardown func()
			)

			BeforeEach(func() {
				fake = newConsumerFakeServer([]*pb.SubscribeResponse{
					cannedMsg("id-1", "test-topic", []byte("payload-1")),
					cannedMsg("id-2", "test-topic", []byte("payload-2")),
				})
				consumer, teardown = startConsumerServer(fake)
			})

			AfterEach(func() { teardown() })

			It("delivers each message with the correct fields to the handler", func() {
				ctx, cancel := context.WithCancel(context.Background())

				var (
					mu       sync.Mutex
					received []*queueti.Message
				)

				go func() {
					_ = consumer.Consume(ctx, func(_ context.Context, msg *queueti.Message) error {
						mu.Lock()
						received = append(received, msg)
						mu.Unlock()
						return nil
					})
				}()

				Eventually(func() int {
					mu.Lock()
					defer mu.Unlock()
					return len(received)
				}, 3*time.Second, 50*time.Millisecond).Should(Equal(2))

				cancel()

				mu.Lock()
				defer mu.Unlock()

				Expect(received[0].ID).To(Equal("id-1"))
				Expect(received[0].Topic).To(Equal("test-topic"))
				Expect(received[0].Payload).To(Equal([]byte("payload-1")))
				Expect(received[0].Metadata).To(HaveKeyWithValue("k", "v"))
				Expect(received[0].RetryCount).To(Equal(2))
				Expect(received[0].CreatedAt).To(Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)))

				Expect(received[1].ID).To(Equal("id-2"))
			})
		})

		Context("when the handler returns nil", func() {
			var (
				fake     *consumerFakeServer
				consumer *queueti.Consumer
				teardown func()
			)

			BeforeEach(func() {
				fake = newConsumerFakeServer([]*pb.SubscribeResponse{
					cannedMsg("msg-ack-1", "test-topic", []byte("data")),
				})
				consumer, teardown = startConsumerServer(fake)
			})

			AfterEach(func() { teardown() })

			It("calls Ack for the message", func() {
				ctx, cancel := context.WithCancel(context.Background())

				go func() {
					_ = consumer.Consume(ctx, func(_ context.Context, _ *queueti.Message) error {
						return nil
					})
				}()

				Eventually(func() []string {
					return fake.ackedIDs()
				}, 3*time.Second, 50*time.Millisecond).Should(ConsistOf("msg-ack-1"))

				cancel()
				Expect(fake.nackedIDs()).To(BeEmpty())
			})
		})

		Context("when the handler returns an error", func() {
			var (
				fake     *consumerFakeServer
				consumer *queueti.Consumer
				teardown func()
			)

			BeforeEach(func() {
				fake = newConsumerFakeServer([]*pb.SubscribeResponse{
					cannedMsg("msg-nack-1", "test-topic", []byte("data")),
				})
				consumer, teardown = startConsumerServer(fake)
			})

			AfterEach(func() { teardown() })

			It("calls Nack with the error string as the reason", func() {
				ctx, cancel := context.WithCancel(context.Background())

				go func() {
					_ = consumer.Consume(ctx, func(_ context.Context, _ *queueti.Message) error {
						return errors.New("processing failed")
					})
				}()

				Eventually(func() []string {
					return fake.nackedIDs()
				}, 3*time.Second, 50*time.Millisecond).Should(ConsistOf("msg-nack-1"))

				cancel()
				Expect(fake.ackedIDs()).To(BeEmpty())
			})
		})

		Context("when the handler panics", func() {
			var (
				fake     *consumerFakeServer
				consumer *queueti.Consumer
				teardown func()
			)

			BeforeEach(func() {
				fake = newConsumerFakeServer([]*pb.SubscribeResponse{
					cannedMsg("msg-panic-1", "test-topic", []byte("data")),
				})
				consumer, teardown = startConsumerServer(fake)
			})

			AfterEach(func() { teardown() })

			It("recovers the panic and Nacks the message", func() {
				ctx, cancel := context.WithCancel(context.Background())

				go func() {
					_ = consumer.Consume(ctx, func(_ context.Context, _ *queueti.Message) error {
						panic("something terrible happened")
					})
				}()

				Eventually(func() []string {
					return fake.nackedIDs()
				}, 3*time.Second, 50*time.Millisecond).Should(ConsistOf("msg-panic-1"))

				cancel()
				Expect(fake.ackedIDs()).To(BeEmpty())
			})
		})

		Context("when ctx is cancelled before messages arrive", func() {
			var (
				fake     *consumerFakeServer
				consumer *queueti.Consumer
				teardown func()
			)

			BeforeEach(func() {
				// No messages — the stream will just block.
				fake = newConsumerFakeServer(nil)
				consumer, teardown = startConsumerServer(fake)
			})

			AfterEach(func() { teardown() })

			It("returns nil when the context is cancelled", func() {
				ctx, cancel := context.WithCancel(context.Background())

				consumeDone := make(chan error, 1)
				go func() {
					consumeDone <- consumer.Consume(ctx, func(_ context.Context, _ *queueti.Message) error {
						return nil
					})
				}()

				// Wait until the stream is up, then cancel.
				Eventually(func() bool {
					select {
					case <-fake.streamReady:
						return true
					default:
						return false
					}
				}, 3*time.Second, 50*time.Millisecond).Should(BeTrue())

				cancel()

				var consumeErr error
				Eventually(consumeDone, 3*time.Second).Should(Receive(&consumeErr))
				Expect(consumeErr).NotTo(HaveOccurred())
			})
		})
	})
})
