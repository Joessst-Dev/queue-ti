package server_test

import (
	"context"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/server"
	pb "github.com/Joessst-Dev/queue-ti/pb"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = Describe("gRPC Server", func() {
	var (
		pool   *pgxpool.Pool
		client pb.QueueServiceClient
		conn   *grpc.ClientConn
		srv    *grpc.Server
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		dsn := "postgres://postgres:postgres@localhost:5432/queueti_test?sslmode=disable"
		pool, err = pgxpool.New(ctx, dsn)
		Expect(err).NotTo(HaveOccurred())

		err = db.Migrate(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		// Ensure a clean state before each test
		_, err = pool.Exec(ctx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())

		queueService := queue.NewService(pool, 30*time.Second)
		grpcServer := server.NewGRPCServer(queueService)

		lis := bufconn.Listen(1024 * 1024)
		srv = grpc.NewServer()
		pb.RegisterQueueServiceServer(srv, grpcServer)

		go func() {
			_ = srv.Serve(lis)
		}()

		conn, err = grpc.NewClient("passthrough:///bufconn",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		Expect(err).NotTo(HaveOccurred())

		client = pb.NewQueueServiceClient(conn)
	})

	AfterEach(func() {
		if conn != nil {
			conn.Close()
		}
		if srv != nil {
			srv.Stop()
		}
		if pool != nil {
			pool.Close()
		}
	})

	// Tests for the Enqueue RPC endpoint
	Describe("Enqueue", func() {
		Context("Given a valid enqueue request with topic and payload", func() {
			It("should successfully enqueue the message and return a unique ID", func() {
				// When we call Enqueue with a valid topic and payload
				resp, err := client.Enqueue(ctx, &pb.EnqueueRequest{
					Topic:   "grpc-topic",
					Payload: []byte("hello grpc"),
				})

				// Then no error occurs and a message ID is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Id).NotTo(BeEmpty())
			})
		})

		Context("Given an enqueue request with an empty topic", func() {
			It("should reject the request with InvalidArgument", func() {
				// When we call Enqueue without a topic
				_, err := client.Enqueue(ctx, &pb.EnqueueRequest{
					Topic:   "",
					Payload: []byte("data"),
				})

				// Then an InvalidArgument gRPC error is returned
				Expect(err).To(HaveOccurred())
				st, _ := status.FromError(err)
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
			})
		})
	})

	// Tests for the Dequeue RPC endpoint
	Describe("Dequeue", func() {
		Context("Given a message has been enqueued on a topic", func() {
			BeforeEach(func() {
				// Given a message with metadata exists on the topic
				_, err := client.Enqueue(ctx, &pb.EnqueueRequest{
					Topic:    "grpc-topic",
					Payload:  []byte("dequeue me"),
					Metadata: map[string]string{"k": "v"},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the message with its payload and metadata", func() {
				// When we dequeue from the topic
				resp, err := client.Dequeue(ctx, &pb.DequeueRequest{Topic: "grpc-topic"})

				// Then the message payload and metadata are correct
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Payload).To(Equal([]byte("dequeue me")))
				Expect(resp.Metadata["k"]).To(Equal("v"))
			})
		})

		Context("Given no messages exist on the requested topic", func() {
			It("should return a NotFound gRPC error", func() {
				// When we attempt to dequeue from an empty topic
				_, err := client.Dequeue(ctx, &pb.DequeueRequest{Topic: "empty"})

				// Then a NotFound error is returned
				Expect(err).To(HaveOccurred())
				st, _ := status.FromError(err)
				Expect(st.Code()).To(Equal(codes.NotFound))
			})
		})
	})

	// Tests for the Ack RPC endpoint
	Describe("Ack", func() {
		Context("Given a message has been dequeued and is being processed", func() {
			var messageID string

			BeforeEach(func() {
				// Given a message is enqueued and then dequeued
				enqResp, err := client.Enqueue(ctx, &pb.EnqueueRequest{
					Topic:   "grpc-topic",
					Payload: []byte("ack me"),
				})
				Expect(err).NotTo(HaveOccurred())
				messageID = enqResp.Id

				_, err = client.Dequeue(ctx, &pb.DequeueRequest{Topic: "grpc-topic"})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should successfully acknowledge and remove the message", func() {
				// When we acknowledge the message by ID
				_, err := client.Ack(ctx, &pb.AckRequest{Id: messageID})

				// Then no error occurs and the message is permanently removed
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

