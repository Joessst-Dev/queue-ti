# queue-ti Python Client

A Python client library for [queue-ti](https://github.com/Joessst-Dev/queue-ti), providing high-level Producer and Consumer APIs for async and sync applications.

- **Async first** — Native async/await with automatic reconnect and token refresh
- **Sync wrapper** — Drop-in synchronous API that runs async code on a background thread
- **Type-safe** — Full type hints and mypy-strict compatible
- **Python 3.11+** — Requires Python 3.11 or later

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
    # Connect to the server
    client = await connect("localhost:50051", options=ConnectOptions(insecure=True))
    producer = client.producer()
    
    # Publish a message
    msg_id = await producer.publish(
        topic="orders",
        payload=b'{"amount": 99.99}',
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
    client = await connect("localhost:50051", options=ConnectOptions(insecure=True))
    
    # Consume messages (blocks until cancelled)
    consumer = client.consumer(
        topic="orders",
        options=ConsumerOptions(concurrency=4),
    )
    
    async def handler(msg):
        print(f"[{msg.id}] {msg.payload}")
        # Return normally to auto-ack; raise to auto-nack
    
    await consumer.consume(handler)

asyncio.run(main())
```

### Sync Producer

```python
from queueti import connect_sync, ConnectOptions

client = connect_sync("localhost:50051", options=ConnectOptions(insecure=True))
producer = client.producer()

msg_id = producer.publish(
    topic="orders",
    payload=b'{"amount": 99.99}',
)
print(f"Published: {msg_id}")

client.close()
```

### Sync Consumer

```python
from queueti import connect_sync, ConnectOptions, ConsumerOptions

client = connect_sync("localhost:50051", options=ConnectOptions(insecure=True))

consumer = client.consumer(
    topic="orders",
    options=ConsumerOptions(concurrency=4),
)

def handler(msg):
    print(f"[{msg.id}] {msg.payload}")
    # Return normally to auto-ack; raise to auto-nack

# Blocks until interrupted (Ctrl+C)
consumer.consume(handler)
```

## Connection

### Basic Connection

```python
from queueti import connect

client = await connect("localhost:50051")
```

### Insecure (Development)

```python
from queueti import connect, ConnectOptions

client = await connect(
    "localhost:50051",
    options=ConnectOptions(insecure=True),
)
```

### With Bearer Token

```python
from queueti import connect, ConnectOptions

client = await connect(
    "localhost:50051",
    options=ConnectOptions(token="your-jwt-token"),
)
```

### With Token Refresh

When your token expires, you can provide a refresher function to obtain a new token automatically:

```python
from queueti import connect, ConnectOptions

async def refresh_token() -> str:
    # Fetch a new token (e.g., from your auth service)
    new_token = await fetch_fresh_token()
    return new_token

client = await connect(
    "localhost:50051",
    options=ConnectOptions(
        token="initial-token",
        token_refresher=refresh_token,
    ),
)

# Token will refresh automatically before expiry
```

You can also manually set a new token:

```python
client.set_token("new-token")
```

### ConnectOptions

All fields are optional.

| Field | Type | Description |
|-------|------|-------------|
| `token` | `str \| None` | Bearer token for JWT authentication |
| `token_refresher` | `Callable[[], Awaitable[str]] \| None` | Async function to refresh the token before expiry |
| `insecure` | `bool` | Disable TLS (for development only; default: `False`) |

## Producer API

### AsyncProducer.publish()

```python
msg_id: str = await producer.publish(
    topic: str,
    payload: bytes,
    options: PublishOptions | None = None,
) -> str
```

Publish a message and return its assigned ID.

**Parameters:**
- `topic` (str) — Topic name
- `payload` (bytes) — Message content
- `options` (PublishOptions | None) — Optional metadata and publishing settings

**Returns:** Message ID (str)

**Raises:** `PublishError` if the RPC fails

**Example:**

```python
msg_id = await producer.publish(
    topic="orders",
    payload=b'{"sku": "ABC123", "qty": 5}',
    options=PublishOptions(metadata={"source": "api"}),
)
```

### Producer.publish() (Sync)

Identical to `AsyncProducer.publish()` but blocks instead of awaiting.

### PublishOptions

| Field | Type | Description |
|-------|------|-------------|
| `metadata` | `dict[str, str]` | Optional metadata key-value pairs (default: `{}`) |

## Consumer API

### AsyncConsumer.consume()

```python
await consumer.consume(
    handler: Callable[[Message], Awaitable[None]],
) -> None
```

Stream messages from the topic, calling the handler for each message. Runs until cancelled. Auto-acks on success; auto-nacks on exception.

**Parameters:**
- `handler` — Async function called for each message. Raise an exception to nack.

**Behavior:**
- Reconnects with exponential backoff on stream errors
- Concurrency controlled via `ConsumerOptions.concurrency`
- Visibility timeout overridable per-call via `ConsumerOptions.visibility_timeout_seconds`

**Example:**

```python
async def process_order(msg: Message):
    order = json.loads(msg.payload)
    print(f"Processing order {order['id']} (retry #{msg.retry_count})")
    if order["amount"] < 0:
        raise ValueError("invalid amount")

await consumer.consume(process_order)
```

### AsyncConsumer.consume_batch()

```python
await consumer.consume_batch(
    options: BatchOptions,
    handler: Callable[[list[Message]], Awaitable[None]],
) -> None
```

Poll batches from the topic, calling the handler with all messages in the batch. Runs until cancelled. Handler is responsible for acking/nacking each message.

**Parameters:**
- `options` (BatchOptions) — Batch size and optional visibility timeout override
- `handler` — Async function called with a list of `Message` objects

**Behavior:**
- Polls with exponential backoff when the queue is empty
- Each message is individually locked and can be acked/nacked independently
- Handler errors do not prevent ack/nack of individual messages

**Example:**

```python
from queueti import BatchOptions

async def handle_batch(messages: list[Message]):
    for msg in messages:
        try:
            order = json.loads(msg.payload)
            await process_order(order)
            await msg.ack()
        except Exception as e:
            await msg.nack(f"processing failed: {e}")

await consumer.consume_batch(
    options=BatchOptions(batch_size=10, visibility_timeout_seconds=60),
    handler=handle_batch,
)
```

### Consumer.consume() (Sync)

Blocks on the calling thread and processes messages one at a time. Identical behavior to async version.

```python
def handler(msg: SyncMessage):
    # Process message; raise to nack
    pass

consumer.consume(handler)  # Blocks until interrupted
```

### Consumer.consume_batch() (Sync)

Blocks on the calling thread and processes message batches.

```python
def handler(messages: list[SyncMessage]):
    for msg in messages:
        try:
            # Process...
            msg.ack()
        except Exception as e:
            msg.nack(f"error: {e}")

consumer.consume_batch(
    options=BatchOptions(batch_size=10),
    handler=handler,
)
```

### ConsumerOptions

All fields are optional.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `concurrency` | `int` | `1` | Number of messages to process in parallel (must be ≥ 1) |
| `visibility_timeout_seconds` | `int \| None` | `None` | Override server default visibility timeout for this consumer (seconds) |
| `consumer_group` | `str \| None` | `None` | Consumer group name for independent message consumption; see [Consumer Groups](#consumer-groups) |

### BatchOptions

| Field | Type | Description |
|-------|------|-------------|
| `batch_size` | `int` | Maximum messages to dequeue in one call (must be ≥ 1) |
| `visibility_timeout_seconds` | `int \| None` | Optional visibility timeout override (seconds) |
| `consumer_group` | `str \| None` | Consumer group name for independent message consumption |

## Message

### Fields

Received from `consume()` or `consume_batch()`.

| Field | Type | Description |
|-------|------|-------------|
| `id` | `str` | Unique message identifier |
| `topic` | `str` | Topic the message belongs to |
| `payload` | `bytes` | Message content |
| `metadata` | `dict[str, str]` | User-supplied metadata |
| `created_at` | `datetime` | Enqueue timestamp (UTC) |
| `retry_count` | `int` | Current retry count (0 = first attempt) |

### Methods

**`await msg.ack() -> None`** — Acknowledge the message (removes it from the queue).

Raises `AckError` if the RPC fails.

**`await msg.nack(reason: str = "") -> None`** — Return the message to the queue for retry (or to DLQ if max retries exceeded).

Raises `NackError` if the RPC fails.

**Note:** When using `consume()`, ack/nack are called automatically. Only call them directly with `consume_batch()`.

### SyncMessage

Identical to `Message`, but with synchronous `ack()` and `nack()` methods. Used with `Consumer.consume()` and `Consumer.consume_batch()`.

## Consumer Groups

Consumer groups enable independent consumption of the same messages by multiple systems. Each group tracks its own delivery state, allowing parallel processing of the same message by different applications without interference.

When a consumer group is specified, the client sends all RPCs scoped to that group and receives all messages enqueued to the topic. Each message is delivered independently to each group. A message is only deleted from the queue when **all** registered groups have acknowledged it.

### Registering a Consumer Group

Consumer groups must be registered on the server before use:

```bash
curl -X POST http://localhost:8080/api/topics/orders/consumer-groups \
  -H "Content-Type: application/json" \
  -d '{"consumer_group": "warehouse"}'
```

Once registered, the group automatically receives all pending messages enqueued before registration (backfill), plus all future messages.

### Async Consumer

```python
import asyncio
from queueti import connect, ConnectOptions, ConsumerOptions

async def main():
    client = await connect("localhost:50051", options=ConnectOptions(insecure=True))
    consumer = client.consumer(
        "orders",
        options=ConsumerOptions(consumer_group="warehouse", concurrency=4),
    )
    
    async def handler(msg):
        print(f"[warehouse] processing {msg.id}")
        # Return normally to Ack; raise to Nack
    
    await consumer.consume(handler)

asyncio.run(main())
```

### Async Batch Consumer

```python
from queueti import BatchOptions

async def handle_batch(messages):
    for msg in messages:
        try:
            # Process...
            await msg.ack()
        except Exception as e:
            await msg.nack(f"error: {e}")

await consumer.consume_batch(
    options=BatchOptions(batch_size=50, consumer_group="warehouse"),
    handler=handle_batch,
)
```

### Sync Consumer

```python
from queueti import connect_sync, ConnectOptions, ConsumerOptions

client = connect_sync("localhost:50051", options=ConnectOptions(insecure=True))
consumer = client.consumer(
    "orders",
    options=ConsumerOptions(consumer_group="warehouse", concurrency=4),
)

def handler(msg):
    print(f"[warehouse] processing {msg.id}")
    # Return normally to Ack; raise to Nack

consumer.consume(handler)  # Blocks until interrupted
```

### Sync Batch Consumer

```python
from queueti import BatchOptions

def handle_batch(messages):
    for msg in messages:
        try:
            # Process...
            msg.ack()
        except Exception as e:
            msg.nack(f"error: {e}")

consumer.consume_batch(
    options=BatchOptions(batch_size=50, consumer_group="warehouse"),
    handler=handle_batch,
)
```

## Error Handling

All exceptions inherit from `QueueTiError`.

### QueueTiError

Base exception for all queue-ti client errors.

```python
from queueti import QueueTiError

try:
    await consumer.consume(handler)
except QueueTiError as e:
    print(f"Queue operation failed: {e}")
```

### PublishError

Raised when a message cannot be published.

```python
from queueti import PublishError

try:
    msg_id = await producer.publish("orders", payload)
except PublishError as e:
    print(f"Failed to publish: {e}")
```

### AckError

Raised when acknowledging a message fails.

```python
from queueti import AckError

try:
    await msg.ack()
except AckError as e:
    print(f"Failed to ack message {msg.id}: {e}")
```

### NackError

Raised when nacking a message fails.

```python
from queueti import NackError

try:
    await msg.nack("processing failed")
except NackError as e:
    print(f"Failed to nack message {msg.id}: {e}")
```

## Examples

### Robust Async Consumer with Exponential Backoff

```python
import asyncio
from queueti import connect, ConnectOptions, ConsumerOptions, Message

async def consume_with_backoff():
    client = await connect(
        "localhost:50051",
        options=ConnectOptions(insecure=True),
    )
    
    consumer = client.consumer(
        topic="emails",
        options=ConsumerOptions(concurrency=8),
    )
    
    async def send_email(msg: Message):
        payload = json.loads(msg.payload)
        try:
            await send_smtp(payload["to"], payload["body"])
        except TemporaryFailure:
            raise  # Nack; will retry after visibility timeout
        except PermanentFailure:
            # Don't raise; let it go to DLQ if max retries exceeded
            await msg.nack("permanent failure, skipping")
    
    try:
        await consumer.consume(send_email)
    except KeyboardInterrupt:
        print("Shutting down...")
    finally:
        await client.close()

asyncio.run(consume_with_backoff())
```

### Batch Processing with Manual Ack/Nack

```python
import asyncio
import json
from queueti import connect, ConnectOptions, BatchOptions, Message

async def batch_processor():
    client = await connect("localhost:50051", options=ConnectOptions(insecure=True))
    consumer = client.consumer("events")
    
    async def process_batch(messages: list[Message]):
        # Process all messages; commit to DB once
        rows = []
        for msg in messages:
            event = json.loads(msg.payload)
            rows.append(event)
        
        try:
            async with db_pool.acquire() as conn:
                await conn.executemany(
                    "INSERT INTO events (...) VALUES (...)",
                    rows,
                )
            # Commit succeeded; ack all
            for msg in messages:
                await msg.ack()
        except Exception as e:
            # Commit failed; nack all
            for msg in messages:
                await msg.nack(f"db error: {e}")
    
    await consumer.consume_batch(
        options=BatchOptions(batch_size=100),
        handler=process_batch,
    )

asyncio.run(batch_processor())
```

### Sync Consumer in a Worker Thread

```python
import threading
import json
from queueti import connect_sync, ConnectOptions, SyncMessage

def worker():
    client = connect_sync("localhost:50051", options=ConnectOptions(insecure=True))
    consumer = client.consumer("webhooks")
    
    def handle_webhook(msg: SyncMessage):
        payload = json.loads(msg.payload)
        requests.post(payload["url"], json=payload["data"])
    
    try:
        consumer.consume(handle_webhook)
    finally:
        client.close()

# Run in a separate thread
thread = threading.Thread(target=worker, daemon=True)
thread.start()
thread.join()
```

## Development setup

macOS and some Linux distributions ship an externally-managed Python that blocks
`pip install` at the system level. Use a virtual environment:

```bash
# From the repo root — creates .venv and installs all dev dependencies
make setup-python

# Run the test suite
make test-python
```

Or manually:

```bash
cd clients/python
python3 -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"
```

## Testing

With the virtual environment active:

```bash
# Run all tests
pytest

# Run specific test file
pytest tests/test_consumer.py

# Run with verbose output
pytest -v

# Run mypy
mypy queueti/
```

## Logging

The library uses Python's standard `logging` module. To see internal debug logs:

```python
import logging

logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger("queueti")
logger.setLevel(logging.DEBUG)
```

## Troubleshooting

### Connection refused

Ensure the queue-ti server is running on the correct host and port:

```python
# Development (insecure, local)
client = await connect("localhost:50051", options=ConnectOptions(insecure=True))

# Production (TLS required)
client = await connect("queue-ti.example.com:50051")
```

### Token refresh not working

Ensure your `token_refresher` function returns a valid JWT string and handles errors:

```python
async def refresh_token() -> str:
    try:
        response = await auth_service.refresh()
        return response.token
    except Exception as e:
        logger.error(f"Token refresh failed: {e}")
        raise  # Exponential backoff will apply
```

### Messages not being processed

Check that:
1. Messages are being published (`publish()` succeeded)
2. Consumer handler is not raising unexpected exceptions
3. Visibility timeout is long enough for your processing (increase via `ConsumerOptions.visibility_timeout_seconds`)
4. Topic exists and has messages (use admin UI or check logs)

## License

MIT
