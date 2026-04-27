// Package queueti provides a Producer/Consumer client library for the
// queue-ti gRPC message queue service.
package queueti

import (
	pb "github.com/Joessst-Dev/queue-ti/pb"
	"google.golang.org/grpc"
)

// Client holds the gRPC connection and the generated service stub.
type Client struct {
	conn *grpc.ClientConn
	pb   pb.QueueServiceClient
}

// Dial connects to the queue-ti gRPC server at addr.
// At least one option (e.g. WithInsecure) must be supplied for the
// transport credentials; the underlying grpc.NewClient call will fail
// otherwise.
func Dial(addr string, opts ...dialOption) (*Client, error) {
	cfg := &dialConfig{}
	for _, o := range opts {
		o(cfg)
	}

	conn, err := grpc.NewClient(addr, cfg.grpcOpts...)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn: conn,
		pb:   pb.NewQueueServiceClient(conn),
	}, nil
}

// NewProducer returns a Producer bound to this client.
func (c *Client) NewProducer() *Producer {
	return &Producer{client: c}
}

// NewConsumer returns a Consumer for the given topic bound to this client.
func (c *Client) NewConsumer(topic string, opts ...ConsumerOption) *Consumer {
	cfg := consumerConfig{
		concurrency: 1,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &Consumer{
		client: c,
		topic:  topic,
		cfg:    cfg,
	}
}

// DialConn wraps an already-established *grpc.ClientConn in a Client.
// This is primarily useful in tests where a bufconn connection is built
// outside of Dial.
func DialConn(conn *grpc.ClientConn) (*Client, error) {
	return &Client{
		conn: conn,
		pb:   pb.NewQueueServiceClient(conn),
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}
