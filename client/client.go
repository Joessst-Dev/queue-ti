// Package queueti provides a Producer/Consumer client library for the
// queue-ti gRPC message queue service.
package queueti

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/Joessst-Dev/queue-ti/pb"
	"google.golang.org/grpc"
)

// Client holds the gRPC connection and the generated service stub.
type Client struct {
	conn        *grpc.ClientConn
	pb          pb.QueueServiceClient
	store       *tokenStore    // nil when auth is disabled
	stopRefresh context.CancelFunc
}

// Dial connects to the queue-ti gRPC server at addr.
// At least one option (e.g. WithInsecure) must be supplied for the
// transport credentials; the underlying grpc.NewClient call will fail
// otherwise.
func Dial(addr string, opts ...DialOption) (*Client, error) {
	cfg := &dialConfig{}
	for _, o := range opts {
		o(cfg)
	}

	conn, err := grpc.NewClient(addr, cfg.grpcOpts...)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:  conn,
		pb:    pb.NewQueueServiceClient(conn),
		store: cfg.store,
	}

	if cfg.refresher != nil && cfg.store != nil {
		refreshCtx, cancel := context.WithCancel(context.Background())
		c.stopRefresh = cancel
		go c.runRefresher(refreshCtx, cfg.refresher)
	}

	return c, nil
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

// Close stops the background token refresher (if any) and closes the
// underlying gRPC connection.
func (c *Client) Close() error {
	if c.stopRefresh != nil {
		c.stopRefresh()
	}
	return c.conn.Close()
}

// SetToken replaces the JWT used for authentication on this connection.
// Takes effect on the next RPC call; no reconnection needed.
// Returns an error if this client was not created with WithBearerToken.
func (c *Client) SetToken(token string) error {
	if c.store == nil {
		return fmt.Errorf("SetToken: client was not created with WithBearerToken")
	}
	c.store.set(token)
	return nil
}

// runRefresher runs until ctx is cancelled. It reads the exp claim of the
// current token, sleeps until 60 s before expiry, calls refresher, and
// updates the store on success. On repeated failure it retries with backoff.
func (c *Client) runRefresher(ctx context.Context, refresher TokenRefresher) {
	const advanceWindow = 60 * time.Second
	retryBackoff := 5 * time.Second

	for {
		exp, err := tokenExpiry(c.store.get())
		var sleepUntil time.Duration
		if err != nil {
			// Can't parse expiry — retry after backoff.
			log.Printf("queue-ti: token refresher: cannot parse token expiry: %v", err)
			sleepUntil = retryBackoff
		} else {
			remaining := time.Until(exp) - advanceWindow
			if remaining <= 0 {
				// Token already expired or within the advance window — refresh now.
				sleepUntil = 0
			} else {
				sleepUntil = remaining
				retryBackoff = 5 * time.Second // reset on healthy token
			}
		}

		if sleepUntil > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleepUntil):
			}
		}

		if ctx.Err() != nil {
			return
		}

		newToken, err := refresher(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("queue-ti: token refresher: refresh failed (retrying in %s): %v", retryBackoff, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryBackoff):
			}
			retryBackoff = min(retryBackoff*2, 60*time.Second)
			continue
		}

		c.store.set(newToken)
		log.Printf("queue-ti: token refresher: token refreshed successfully")
		retryBackoff = 5 * time.Second
	}
}
