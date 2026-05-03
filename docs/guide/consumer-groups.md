# Consumer Groups

Consumer groups enable independent consumption of the same messages by multiple systems. Without consumer groups, a single dequeue operation removes a message from a topic for all consumers. With consumer groups, each group independently tracks the processing state of every message, allowing the same message to be delivered and processed by multiple consumer systems in parallel.

## Key Behaviors

- **Group independence** — Each group maintains its own delivery state for every message. Acking a message in one group does not affect other groups.
- **Parallel processing** — Multiple groups can process the same message concurrently without blocking each other.
- **Message lifecycle per group** — A message is deleted from the queue only when **all** registered groups have acknowledged it. If any group has not yet processed (or nacked) a message, it remains available.
- **Legacy mode** — When using the default consumer group (or no group specified in older client versions), queue-ti behaves as a single-consumer queue, maintaining backward compatibility.

## Registering a Consumer Group

Register a group via the HTTP admin API:

```bash
# Register a new group for a topic
curl -X POST http://localhost:8080/api/topics/orders/consumer-groups \
  -H "Content-Type: application/json" \
  -d '{"consumer_group": "warehouse"}'

# List all groups for a topic
curl http://localhost:8080/api/topics/orders/consumer-groups

# Unregister a group
curl -X DELETE http://localhost:8080/api/topics/orders/consumer-groups/warehouse
```

Once registered, a group automatically receives all pending messages that were enqueued before registration (backfill). Future messages are delivered to all registered groups.

## Using Consumer Groups in Client Libraries

### Go Client

```go
consumer := client.NewConsumer("orders",
    queueti.WithConsumerGroup("warehouse"),
    queueti.WithConcurrency(4),
)

err := consumer.Consume(ctx, func(ctx context.Context, msg *queueti.Message) error {
    // Process message...
    return nil // Ack; return error to Nack
})
```

For batch consumption:

```go
consumer := client.NewConsumer("orders",
    queueti.WithConsumerGroup("warehouse"),
)

err := consumer.ConsumeBatch(ctx, "orders", 50,
    func(ctx context.Context, messages []*queueti.Message) error {
        for _, msg := range messages {
            // Process...
            msg.Ack(ctx)
        }
        return nil
    },
)
```

### Node.js Client

```typescript
const consumer = client.consumer('orders', {
  consumerGroup: 'warehouse',
  concurrency: 4,
})

await consumer.consume(async (msg) => {
  // Process message...
  // Return normally to Ack; throw to Nack
})
```

For batch consumption:

```typescript
await consumer.consumeBatch(
  { batchSize: 50, consumerGroup: 'warehouse' },
  async (messages) => {
    for (const msg of messages) {
      // Process...
      await msg.ack()
    }
  },
)
```

### Python Client

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
        # Process message...
        pass
    
    await consumer.consume(handler)

asyncio.run(main())
```

For batch consumption:

```python
from queueti import BatchOptions

async def handle_batch(messages):
    for msg in messages:
        # Process...
        await msg.ack()

await consumer.consume_batch(
    options=BatchOptions(batch_size=50, consumer_group="warehouse"),
    handler=handle_batch,
)
```

Sync variant:

```python
from queueti import connect_sync, ConnectOptions, ConsumerOptions

client = connect_sync("localhost:50051", options=ConnectOptions(insecure=True))
consumer = client.consumer(
    "orders",
    options=ConsumerOptions(consumer_group="warehouse"),
)

consumer.consume(handler)  # Blocks until interrupted
```

## Consumer Group Grants

Consumer group grants let you restrict a user to a specific named consumer group on a topic. This is useful when multiple teams consume from the same topic and you want to enforce that each team only processes its own group.

See the [Authentication](./authentication#consumer-group-grants) guide for detailed information on consumer group grants and how to create them.
