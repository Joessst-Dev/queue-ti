// Order pipeline — demonstrates the full producer → consumer → ack lifecycle
// against a local queue-ti instance.
//
// Run: go run . (requires docker-compose up)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"
)

const (
	grpcAddr      = "localhost:50051"
	adminAddr     = "http://localhost:8080"
	topic         = "orders"
	dlqTopic      = "orders.dlq"
	consumerGroup = "fulfillment"
)

type order struct {
	ID     string `json:"id"`
	Item   string `json:"item"`
	Amount int    `json:"amount"`
	Poison bool   `json:"poison,omitempty"` // triggers a nack to simulate failure
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	auth, err := queueti.NewAuth(adminAddr, envOr("QUEUETI_USERNAME", "admin"), envOr("QUEUETI_PASSWORD", "secret"))
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	dialOpts := []queueti.DialOption{queueti.WithInsecure()}
	if auth.Token() != "" {
		dialOpts = append(dialOpts,
			queueti.WithBearerToken(auth.Token()),
			queueti.WithTokenRefresher(auth.Refresh),
		)
	}

	client, err := queueti.Dial(grpcAddr, dialOpts...)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()

	admin := queueti.NewAdminClient(adminAddr, queueti.WithAdminToken(auth.Token()))

	if err := registerConsumerGroup(ctx, admin); err != nil {
		log.Fatalf("register consumer group: %v", err)
	}

	go produce(ctx, client)
	go drainDLQ(ctx, client)
	consume(ctx, client)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// registerConsumerGroup ensures the consumer group exists via the admin API.
func registerConsumerGroup(ctx context.Context, admin *queueti.AdminClient) error {
	err := admin.RegisterConsumerGroup(ctx, topic, consumerGroup)
	if err != nil && !errors.Is(err, queueti.ErrConflict) {
		return fmt.Errorf("register consumer group: %w", err)
	}
	log.Printf("consumer group %q ready", consumerGroup)
	return nil
}

// produce publishes five orders — the third is a poison pill that will be nacked.
func produce(ctx context.Context, client *queueti.Client) {
	producer := client.NewProducer()
	orders := []order{
		{ID: "ord-1", Item: "Widget A", Amount: 2},
		{ID: "ord-2", Item: "Gadget B", Amount: 1},
		{ID: "ord-3", Item: "poison", Amount: 0, Poison: true},
		{ID: "ord-4", Item: "Widget C", Amount: 5},
		{ID: "ord-5", Item: "Gadget D", Amount: 3},
	}

	for _, o := range orders {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}

		payload, _ := json.Marshal(o)
		opts := []queueti.PublishOption{
			queueti.WithMetadata(map[string]string{"source": "order-pipeline"}),
			queueti.WithKey(o.ID),
		}
		id, err := producer.Publish(ctx, topic, payload, opts...)
		if err != nil {
			log.Printf("publish %s: %v", o.ID, err)
			continue
		}
		log.Printf("published %s → message %s", o.ID, id)
	}
}

// consume streams from the orders topic; poisons are nacked and eventually land in the DLQ.
func consume(ctx context.Context, client *queueti.Client) {
	consumer := client.NewConsumer(topic,
		queueti.WithConsumerGroup(consumerGroup),
		queueti.WithConcurrency(3),
	)
	log.Printf("consuming from %q (group %q) — Ctrl-C to stop", topic, consumerGroup)

	if err := consumer.Consume(ctx, handleOrder); err != nil {
		log.Printf("consumer stopped: %v", err)
	}
}

func handleOrder(ctx context.Context, msg *queueti.Message) error {
	var o order
	if err := json.Unmarshal(msg.Payload, &o); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	if o.Poison {
		log.Printf("nack %s: poison pill detected (retry %d)", msg.ID, msg.RetryCount)
		return fmt.Errorf("poison pill")
	}

	log.Printf("ack %s: processed order %s — %d×%s", msg.ID, o.ID, o.Amount, o.Item)
	return nil
}

// drainDLQ polls the DLQ and logs dead-lettered messages.
func drainDLQ(ctx context.Context, client *queueti.Client) {
	consumer := client.NewConsumer(dlqTopic, queueti.WithConsumerGroup(consumerGroup))
	log.Printf("draining DLQ %q", dlqTopic)

	_ = consumer.ConsumeBatch(ctx, dlqTopic, 10, func(ctx context.Context, msgs []*queueti.Message) error {
		for _, msg := range msgs {
			log.Printf("[DLQ] %s retry=%d payload=%s", msg.ID, msg.RetryCount, msg.Payload)
			if err := msg.Ack(ctx); err != nil {
				log.Printf("[DLQ] ack failed: %v", err)
			}
		}
		return nil
	})
}
