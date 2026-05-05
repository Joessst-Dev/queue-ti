# Order Pipeline — Node.js

Demonstrates the full producer → consumer → ack lifecycle using the queue-ti Node.js client.

## What it does

1. Registers the `fulfillment` consumer group via the admin REST API
2. Publishes 5 orders to the `orders` topic (one is a poison pill)
3. Consumes the topic with 3 concurrent handlers — valid orders are acked, the poison pill is nacked
4. Drains the `orders.dlq` topic so dead-lettered messages are visible

## Prerequisites

- Node.js 18+
- `ts-node` available (`npm install -g ts-node` or `npx ts-node`)
- queue-ti running locally: `docker-compose up` from the repo root

## Run

From the `clients/node/` directory:

```bash
npm install
npx ts-node --esm examples/order-pipeline/index.ts
```

Press **Ctrl-C** to stop the consumer.

## Configuration

Edit the constants at the top of `index.ts`:

| Constant | Default | Description |
|----------|---------|-------------|
| `GRPC_ADDR` | `localhost:50051` | gRPC server address |
| `ADMIN_URL` | `http://localhost:8080` | HTTP admin API base URL |
| `TOPIC` | `orders` | Topic to publish/consume |
| `DLQ_TOPIC` | `orders.dlq` | Dead-letter queue topic |
| `CONSUMER_GROUP` | `fulfillment` | Consumer group name |
