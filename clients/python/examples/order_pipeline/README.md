# Order Pipeline — Python

Demonstrates the full producer → consumer → ack lifecycle using the queue-ti Python client.

## What it does

1. Registers the `fulfillment` consumer group via the admin REST API
2. Publishes 5 orders to the `orders` topic (one is a poison pill)
3. Consumes the topic with 3 concurrent handlers — valid orders are acked, the poison pill is nacked
4. Drains the `orders.dlq` topic so dead-lettered messages are visible

## Prerequisites

- Python 3.11+
- queue-ti running locally: `docker-compose up` from the repo root

## Install dependencies

From the `clients/python/` directory:

```bash
pip install -e ".[dev]"
```

Or with the project's virtual environment:

```bash
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
```

## Run

```bash
python examples/order_pipeline/main.py
```

Press **Ctrl-C** to stop the consumer.

## Configuration

Edit the constants at the top of `main.py`:

| Constant | Default | Description |
|----------|---------|-------------|
| `GRPC_ADDR` | `localhost:50051` | gRPC server address |
| `ADMIN_URL` | `http://localhost:8080` | HTTP admin API base URL |
| `TOPIC` | `orders` | Topic to publish/consume |
| `DLQ_TOPIC` | `orders.dlq` | Dead-letter queue topic |
| `CONSUMER_GROUP` | `fulfillment` | Consumer group name |
