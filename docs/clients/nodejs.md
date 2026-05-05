# Node.js Client

The `@queue-ti/client` npm package provides TypeScript-first Producer/Consumer APIs for Node.js applications. It connects via gRPC with automatic token refresh, graceful reconnection, and batch consumption support.

## Installation

```bash
npm install @queue-ti/client
```

Or with yarn:

```bash
yarn add @queue-ti/client
```

## Quick Start

### Producer

```typescript
import { connect } from '@queue-ti/client'

const client = await connect('localhost:50051', { insecure: true })
const producer = client.producer()

const id = await producer.publish('orders', Buffer.from(JSON.stringify({ amount: 99.99 })), {
  metadata: { source: 'checkout' },
})
console.log('published:', id)
client.close()
```

### Consumer

```typescript
const consumer = client.consumer('orders', { concurrency: 4 })
const signal = AbortSignal.timeout(60_000)

await consumer.consume(async (msg) => {
  console.log(`[${msg.id}] ${msg.payload.toString()}`)
  // Return normally to Ack; throw to Nack
})
```

### Batch Consumer

```typescript
const consumer = client.consumer('orders')

await consumer.consumeBatch(
  { batchSize: 50 },
  async (messages) => {
    for (const msg of messages) {
      try {
        await processOrder(msg.payload)
        await msg.ack()
      } catch (err) {
        await msg.nack(`Processing failed: ${err.message}`)
      }
    }
  },
)
```

## API Reference

### connect

Establishes a connection to the queue-ti gRPC server.

```typescript
const client = await connect('localhost:50051', {
  insecure: true,                      // Plaintext (no TLS)
  auth?: {
    token: string,                     // Initial JWT token
    refreshToken?: (ctx) => Promise<string>, // Token refresh function
  },
})
```

**Options:**
- `insecure` (boolean) — Use plaintext instead of TLS (for local development)
- `auth.token` (string, optional) — Initial JWT token for auth
- `auth.refreshToken` (async function, optional) — Function to refresh JWT tokens before expiry

### Producer

#### publish

Enqueues a message to a topic.

```typescript
const id = await producer.publish(topic, payload, {
  metadata?: Record<string, string>,  // Optional metadata
  key?: string,                        // Optional deduplication key
})
```

**Parameters:**
- `topic` (string) — Topic name
- `payload` (Buffer) — Message payload
- `options` (optional):
  - `metadata` — Key-value metadata
  - `key` — Deduplication key for upsert semantics

**Return:** Message UUID as string

### Consumer

#### consume

Consumes messages one at a time from a topic.

```typescript
await consumer.consume(async (msg) => {
  console.log(`[${msg.id}] ${msg.payload.toString()}`)
  // Return normally to Ack; throw to Nack
}, {
  signal?: AbortSignal,               // Optional abort signal to stop consuming
})
```

**Message object:**
- `id` (string) — Message UUID
- `topic` (string) — Topic name
- `payload` (Buffer) — Message payload
- `metadata` (Record<string, string>) — Message metadata
- `createdAt` (Date) — Enqueue timestamp
- `retryCount` (number) — Current retry count
- `maxRetries` (number) — Maximum retries allowed
- `key` (string, optional) — Deduplication key (if present)

**Handler:**
- Return normally to acknowledge the message
- Throw an error to nack the message with that error as the reason

**Behavior:**
- Blocks until `signal` is aborted or handler throws a fatal error
- Auto-reconnects on connection loss
- Auto-refreshes JWT tokens before expiry

**Consumer options:**
- `concurrency` (number, default: 1) — Number of parallel dequeue goroutines
- `consumerGroup` (string, optional) — Consumer group name for group-based consumption
- `visibilityTimeout` (number, optional) — Override default visibility timeout (in seconds)

#### consumeBatch

Consumes messages in batches for higher throughput.

```typescript
await consumer.consumeBatch(
  { batchSize: 50 },
  async (messages) => {
    for (const msg of messages) {
      try {
        await processOrder(msg.payload)
        await msg.ack()
      } catch (err) {
        await msg.nack(`Processing failed: ${err.message}`)
      }
    }
  },
  { signal }
)
```

**Batch options:**
- `batchSize` (number) — Number of messages to dequeue (1–1000)
- `consumerGroup` (string, optional) — Consumer group name for group-based consumption
- `visibilityTimeout` (number, optional) — Override default visibility timeout (in seconds)

**BatchMessage object:**
- All message fields from `consume` plus:
- `ack()` (async) — Acknowledge the message (removes it from the queue)
- `nack(reason: string)` (async) — Nack the message (optionally with error reason); triggers retry or DLQ promotion

**Behavior**:
- Dequeues up to `batchSize` messages in a single gRPC call
- Returns immediately with available messages (0 to batchSize); never blocks waiting for more
- Each message in the batch is individually locked and can be acked or nacked independently
- Auto-reconnect and token refresh work the same as single-message `consume`

## Error Handling

```typescript
try {
  await consumer.consume(async (msg) => {
    // Process message
    return msg
  })
} catch (err) {
  if (err.code === 'CANCELLED') {
    console.log('Consumer cancelled')
  } else {
    console.error('Consumer error:', err)
  }
}
```

## Authentication

### With JWT Tokens

```typescript
import { connect } from '@queue-ti/client'

const client = await connect('localhost:50051', {
  insecure: true,
  auth: {
    token: initialToken,
    refreshToken: async () => {
      const response = await fetch('http://localhost:8080/api/auth/refresh', {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${currentToken}` },
      })
      const data = await response.json()
      return data.token
    },
  },
})
```

## Consumer Groups

Use consumer groups to allow multiple independent systems to process the same messages:

```typescript
const warehouse = client.consumer('orders', {
  consumerGroup: 'warehouse',
  concurrency: 4,
})

const analytics = client.consumer('orders', {
  consumerGroup: 'analytics',
  concurrency: 2,
})

// Each group independently processes all messages
```

See [Consumer Groups](../guide/consumer-groups) for details.

## Admin API

The `AdminClient` provides programmatic management of topic configuration, schemas, and consumer groups through the HTTP admin API on port 8080.

### Setup

```typescript
import { AdminClient } from '@queue-ti/client'

const admin = new AdminClient('http://localhost:8080', {
  token: 'your-jwt-token',
})
```

### Example: Topic Configuration

```typescript
// List all topic configs
const configs = await admin.listTopicConfigs()

// Set per-topic overrides
const config = await admin.upsertTopicConfig('orders', {
  max_retries: 5,
  message_ttl_seconds: 3600,
  replayable: true,
})

// Delete a topic config (reverts to defaults)
await admin.deleteTopicConfig('orders')
```

### Error Handling

```typescript
import { AdminError } from '@queue-ti/client'

try {
  await admin.listTopicConfigs()
} catch (err) {
  if (err instanceof AdminError) {
    if (err.statusCode === 404) {
      // Handle HTTP 404
    } else if (err.statusCode === 409) {
      // Handle HTTP 409
    }
  }
}
```

### Full API

The `AdminClient` covers:
- **Topic configs**: `listTopicConfigs()`, `upsertTopicConfig(topic, config)`, `deleteTopicConfig(topic)`
- **Topic schemas**: `listTopicSchemas()`, `getTopicSchema(topic)`, `upsertTopicSchema(topic, schemaJson)`, `deleteTopicSchema(topic)`
- **Consumer groups**: `listConsumerGroups(topic)`, `registerConsumerGroup(topic, group)`, `unregisterConsumerGroup(topic, group)`
- **Statistics**: `stats()`

For complete examples and method signatures, see [clients/node/src/admin.ts](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/node/src/admin.ts).

## Full Client Documentation

For complete API reference and examples, see [clients/node/README.md](https://github.com/Joessst-Dev/queue-ti/tree/main/clients/node).
