# Python Client

The `queue-ti-client` PyPI package provides async-first Producer/Consumer APIs for Python 3.11+ applications. It features automatic token refresh, graceful reconnection, and batch consumption, with both async and synchronous wrapper interfaces.

## Installation

```bash
pip install queue-ti-client
```

## Quick Start

### Async Producer

```python
import asyncio
from queueti import connect, ConnectOptions

async def main():
    client = await connect(
        "localhost:50051",
        options=ConnectOptions(insecure=True),
    )
    producer = client.producer()
    
    msg_id = await producer.publish(
        "orders",
        b'{"amount": 99.99}',
    )
    print(f"Published: {msg_id}")
    await client.close()

asyncio.run(main())
```

### Async Consumer

```python
import asyncio
from queueti import connect, ConnectOptions, ConsumerOptions

async def main():
    client = await connect(
        "localhost:50051",
        options=ConnectOptions(insecure=True),
    )
    consumer = client.consumer(
        "orders",
        options=ConsumerOptions(concurrency=4),
    )
    
    async def handle(msg):
        print(f"[{msg.id}] {msg.payload.decode()}")
        # Return normally to auto-ack; raise to auto-nack
    
    await consumer.consume(handle)

asyncio.run(main())
```

### Synchronous API

For sync-only applications, use `connect_sync()` which runs async operations on a background thread:

```python
from queueti import connect_sync, ConnectOptions

client = connect_sync(
    "localhost:50051",
    options=ConnectOptions(insecure=True),
)
producer = client.producer()
msg_id = producer.publish("orders", b'{"amount": 99.99}')
print(f"Published: {msg_id}")
client.close()
```

## API Reference

### connect

Establishes an async connection to the queue-ti gRPC server.

```python
from queueti import connect, ConnectOptions

client = await connect(
    "localhost:50051",
    options=ConnectOptions(
        insecure=True,                          # Plaintext (no TLS)
        auth_token="<jwt-token>",               # Initial JWT token
        token_refresher=async_refresh_fn,       # Token refresh function
    ),
)
```

**ConnectOptions:**
- `insecure` (bool) — Use plaintext instead of TLS (for local development)
- `auth_token` (str, optional) — Initial JWT token for auth
- `token_refresher` (async callable, optional) — Function to refresh JWT tokens before expiry

### connect_sync

Establishes a synchronous connection (async operations on background thread).

```python
from queueti import connect_sync, ConnectOptions

client = connect_sync(
    "localhost:50051",
    options=ConnectOptions(insecure=True),
)
```

### Producer

#### publish

Enqueues a message to a topic.

```python
msg_id = await producer.publish(
    topic,
    payload,
    metadata={"key": "value"},  # Optional metadata
    key="order-123",            # Optional deduplication key
)
```

**Parameters:**
- `topic` (str) — Topic name
- `payload` (bytes) — Message payload
- `metadata` (dict, optional) — Key-value metadata
- `key` (str, optional) — Deduplication key for upsert semantics

**Return:** Message UUID as string

### Consumer

#### consume

Consumes messages one at a time from a topic.

```python
async def handler(msg):
    print(f"[{msg.id}] {msg.payload.decode()}")
    # Return normally to auto-ack; raise to auto-nack

await consumer.consume(handler)
```

**Message object:**
- `id` (str) — Message UUID
- `topic` (str) — Topic name
- `payload` (bytes) — Message payload
- `metadata` (dict[str, str]) — Message metadata
- `created_at` (datetime) — Enqueue timestamp
- `retry_count` (int) — Current retry count
- `max_retries` (int) — Maximum retries allowed
- `key` (str, optional) — Deduplication key (if present)

**Handler:**
- Return normally to acknowledge the message
- Raise an exception to nack the message with that error as the reason

**Behavior:**
- Blocks until interrupted or handler raises a fatal error
- Auto-reconnects on connection loss
- Auto-refreshes JWT tokens before expiry

**Consumer options:**
- `concurrency` (int, default: 1) — Number of parallel dequeue goroutines
- `consumer_group` (str, optional) — Consumer group name for group-based consumption
- `visibility_timeout` (int, optional) — Override default visibility timeout (in seconds)

#### consume_batch

Consumes messages in batches for higher throughput.

```python
from queueti import BatchOptions

async def handle_batch(messages):
    for msg in messages:
        try:
            await process_order(msg.payload)
            await msg.ack()
        except Exception as err:
            await msg.nack(f"Processing failed: {err}")

await consumer.consume_batch(
    options=BatchOptions(batch_size=50),
    handler=handle_batch,
)
```

**BatchOptions:**
- `batch_size` (int) — Number of messages to dequeue (1–1000)
- `consumer_group` (str, optional) — Consumer group name for group-based consumption
- `visibility_timeout` (int, optional) — Override default visibility timeout (in seconds)

**BatchMessage object:**
- All message fields from `consume` plus:
- `ack()` (async) — Acknowledge the message (removes it from the queue)
- `nack(reason: str)` (async) — Nack the message (optionally with error reason); triggers retry or DLQ promotion

**Behavior**:
- Dequeues up to `batch_size` messages in a single gRPC call
- Returns immediately with available messages (0 to batch_size); never blocks waiting for more
- Each message in the batch is individually locked and can be acked or nacked independently
- Auto-reconnect and token refresh work the same as single-message `consume`

## Synchronous Wrappers

All async APIs have synchronous variants via `connect_sync()`:

```python
from queueti import connect_sync, ConsumerOptions

client = connect_sync("localhost:50051", options=ConnectOptions(insecure=True))

# Synchronous producer
producer = client.producer()
msg_id = producer.publish("orders", b'{"amount": 99.99}')

# Synchronous consumer
consumer = client.consumer("orders", options=ConsumerOptions(concurrency=4))

def handler(msg):
    print(f"[{msg.id}] {msg.payload.decode()}")

consumer.consume(handler)  # Blocks until interrupted
```

## Error Handling

```python
import asyncio
from grpc import RpcError

try:
    await consumer.consume(handler)
except asyncio.CancelledError:
    print("Consumer cancelled")
except RpcError as err:
    print(f"gRPC error: {err.code()}")
except Exception as err:
    print(f"Consumer error: {err}")
```

## Authentication

### Using QueueTiAuth (recommended)

The `QueueTiAuth` helper automatically checks if authentication is required and handles login and token refresh:

```python
import asyncio
import queueti

auth = queueti.QueueTiAuth.login("http://localhost:8080", "admin", "secret")

async def main():
    opts = queueti.ConnectOptions(
        token=auth.token,
        token_refresher=auth.async_refresh,
    )
    async with await queueti.connect("localhost:50051", opts) as client:
        producer = client.producer()
        msg_id = await producer.publish("orders", b"...")
        print(f"Published: {msg_id}")

    admin = queueti.AsyncAdminClient(
        "http://localhost:8080",
        queueti.AdminOptions(token=auth.token),
    )
    configs = await admin.list_topic_configs()
    await admin.close()

asyncio.run(main())
```

The `QueueTiAuth` helper:
1. Calls `GET /api/auth/status` to check if authentication is required
2. If auth is disabled, returns a no-op instance with a null token
3. If auth is enabled, calls `POST /api/auth/login` with the provided credentials
4. Exposes `.token` (str or None) for the current JWT and `.async_refresh()` (async callable) which satisfies the `ConnectOptions.token_refresher` interface for automatic token refresh

### With JWT Tokens (manual)

```python
import asyncio
from queueti import connect, ConnectOptions

async def refresh_token(current_token):
    # Your token refresh logic
    response = await http_client.post(
        "http://localhost:8080/api/auth/refresh",
        headers={"Authorization": f"Bearer {current_token}"},
    )
    data = await response.json()
    return data["token"]

async def main():
    client = await connect(
        "localhost:50051",
        options=ConnectOptions(
            insecure=True,
            auth_token=initial_token,
            token_refresher=refresh_token,
        ),
    )

asyncio.run(main())
```

## Consumer Groups

Use consumer groups to allow multiple independent systems to process the same messages:

```python
from queueti import ConsumerOptions

warehouse = client.consumer(
    "orders",
    options=ConsumerOptions(
        consumer_group="warehouse",
        concurrency=4,
    ),
)

analytics = client.consumer(
    "orders",
    options=ConsumerOptions(
        consumer_group="analytics",
        concurrency=2,
    ),
)

# Each group independently processes all messages
```

See [Consumer Groups](../guide/consumer-groups) for details.

## Admin API

The `AsyncAdminClient` provides programmatic management of topic configuration, schemas, and consumer groups through the HTTP admin API on port 8080.

### Setup

```python
from queueti import AsyncAdminClient, AdminOptions

async with AsyncAdminClient(
    'http://localhost:8080',
    options=AdminOptions(token='your-jwt-token'),
) as admin:
    configs = await admin.list_topic_configs()
```

### Example: Topic Configuration

```python
from queueti import AsyncAdminClient, AdminOptions, TopicConfig

admin = AsyncAdminClient('http://localhost:8080')

# List all topic configs
configs = await admin.list_topic_configs()

# Set per-topic overrides
config = TopicConfig(
    topic='orders',
    max_retries=5,
    message_ttl_seconds=3600,
    replayable=True,
)
result = await admin.upsert_topic_config('orders', config)

# Delete a topic config (reverts to defaults)
await admin.delete_topic_config('orders')

await admin.close()
```

### Error Handling

```python
from queueti import AsyncAdminClient, AdminError

try:
    await admin.list_topic_configs()
except AdminError as err:
    if err.status_code == 404:
        # Handle HTTP 404
        print(f"Not found: {err}")
    elif err.status_code == 409:
        # Handle HTTP 409
        print(f"Conflict: {err}")
```

### Full API

The `AsyncAdminClient` covers:
- **Topic configs**: `list_topic_configs()`, `upsert_topic_config(topic, config)`, `delete_topic_config(topic)`
- **Topic schemas**: `list_topic_schemas()`, `get_topic_schema(topic)`, `upsert_topic_schema(topic, schema_json)`, `delete_topic_schema(topic)`
- **Consumer groups**: `list_consumer_groups(topic)`, `register_consumer_group(topic, group)`, `unregister_consumer_group(topic, group)`
- **Statistics**: `stats()`

For complete examples and method signatures, see [clients/python/queueti/_admin.py](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/python/queueti/_admin.py).

## Sample Applications

### Order Pipeline

A self-contained end-to-end example demonstrating the full producer → consumer → ack lifecycle:

- Client creation and consumer group registration via the admin REST API
- Publishing messages with metadata
- Streaming consumption with `concurrency=3`, ack on success, nack on failure (poison pill)
- DLQ drain — batch-polls `orders.dlq` and acks dead-lettered messages
- Graceful shutdown on SIGINT/SIGTERM via `asyncio.Event` + `loop.add_signal_handler`

**Location**: [`clients/python/examples/order_pipeline/`](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/python/examples/order_pipeline)

```bash
# From clients/python/ — requires: docker-compose up (from repo root)
pip install -e ".[dev]"
python examples/order_pipeline/main.py
```

## Full Client Documentation

For complete API reference and examples, see [clients/python/README.md](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/python).
