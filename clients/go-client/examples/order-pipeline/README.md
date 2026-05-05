# Order Pipeline — Go

Demonstrates the full producer → consumer → ack lifecycle using the queue-ti Go client.

## What it does

1. Registers the `fulfillment` consumer group via the admin REST API
2. Publishes 5 orders to the `orders` topic (one is a poison pill)
3. Consumes the topic with 3 concurrent handlers — valid orders are acked, the poison pill is nacked
4. Drains the `orders.dlq` topic so dead-lettered messages are visible

## Prerequisites

- Go 1.21+
- queue-ti running locally: `docker-compose up` from the repo root

## Run

```bash
go run .
```

Press **Ctrl-C** to stop the consumer.

## Configuration

Edit the constants at the top of `main.go` to point at a different server:

| Constant | Default | Description |
|----------|---------|-------------|
| `grpcAddr` | `localhost:50051` | gRPC server address |
| `adminAddr` | `http://localhost:8080` | HTTP admin API base URL |
| `topic` | `orders` | Topic to publish/consume |
| `dlqTopic` | `orders.dlq` | Dead-letter queue topic |
| `consumerGroup` | `fulfillment` | Consumer group name |
