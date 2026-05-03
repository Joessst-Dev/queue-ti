# @queue-ti/client

A Node.js (TypeScript) client library for producing and consuming messages from a [queue-ti](https://github.com/Joessst-Dev/queue-ti) server.

## Installation

```bash
npm install @queue-ti/client
```

Import as:

```typescript
import { connect } from '@queue-ti/client'
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
console.log('enqueued:', id)

client.close()
```

### Consumer

```typescript
import { connect } from '@queue-ti/client'

const client = await connect('localhost:50051', { insecure: true })
const consumer = client.consumer('orders', { concurrency: 4 })

// Consume until process receives SIGINT
const signal = AbortSignal.timeout(60_000) // Stop after 60s for this example
await consumer.consume(async (msg) => {
  console.log(`[${msg.id}] ${msg.payload.toString()}`)
  // return normally to Ack; throw an error to Nack
})

client.close()
```

---

## Connecting

### `connect(address: string, options?: ConnectOptions): Promise<Client>`

Opens a gRPC connection to the server at `address`.

```typescript
// No auth (local/dev)
const client = await connect('localhost:50051', { insecure: true })

// With JWT authentication and automatic token refresh
const client = await connect('localhost:50051', {
  insecure: true,
  token: initialToken,
  tokenRefresher: async () => {
    const resp = await fetch('http://localhost:8080/api/auth/refresh', {
      method: 'POST',
      headers: { Authorization: `Bearer ${currentToken}` },
    })
    const { token } = await resp.json()
    return token
  },
})
```

Always call `client.close()` when done (typically via `finally` or in a cleanup handler).

### ConnectOptions

| Option | Type | Description |
|---|---|---|
| `insecure?` | boolean | Disable TLS — suitable for local and Docker deployments (default: `false`) |
| `token?` | string | Attach a JWT Bearer token to every RPC call |
| `tokenRefresher?` | `() => Promise<string>` | Callback the library calls automatically to obtain a fresh token before expiry |

---

## Client

### `client.producer(): Producer`

Returns a producer for enqueuing messages.

```typescript
const producer = client.producer()
```

### `client.consumer(topic: string, options?: ConsumerOptions): Consumer`

Returns a consumer for the given topic.

```typescript
const consumer = client.consumer('orders', {
  concurrency: 8,
  visibilityTimeoutSeconds: 60,
})
```

### `client.setToken(token: string): void`

Updates the authentication token on a live connection. The new token takes effect on the next RPC call.

```typescript
client.setToken(newToken)
```

Throws an error if the client was not created with a `token` option.

### `client.close(): void`

Closes the gRPC connection and stops background token refresh (if enabled).

```typescript
client.close()
```

---

## Producing Messages

### `producer.publish(topic: string, payload: Buffer | Uint8Array, options?: PublishOptions): Promise<string>`

Enqueues `payload` on `topic`. Returns the assigned message ID.

```typescript
const id = await producer.publish('payments', Buffer.from('...'))

// With metadata
const id = await producer.publish('payments', payload, {
  metadata: {
    tenant: 'acme',
    trace: traceID,
  },
})

// With a deduplication key (upserts pending messages with same key)
const id = await producer.publish('orders', payload, {
  key: 'order-123',
  metadata: { customer: 'acme' },
})
```

### PublishOptions

| Option | Type | Description |
|---|---|---|
| `metadata?` | `Record<string, string>` | Arbitrary key-value metadata attached to the message |
| `key?` | string | Deduplication key for upsert semantics |

#### Idempotent publishing with `key`

When you publish a message with a key, queue-ti uses upsert semantics: if a pending message with the same topic and key already exists in the queue, its payload and metadata are updated in place (the existing message ID is returned). This ensures idempotency when retrying publish operations.

**Caveat:** Once a message begins processing (transitions to `processing` status), it is no longer considered "pending". A key match only applies to messages awaiting processing. If the keyed message is already processing, a new row is inserted.

```typescript
// Idempotent publish: if a message with topic="orders" and key="order-123" 
// exists and is pending, it is updated. Otherwise a new message is created.
const id = await producer.publish('orders', Buffer.from(`{"amount": 150.00}`), {
  key: 'order-123',
  metadata: { customer: 'acme' },
})
console.log('message id:', id) // same on retry if order-123 is still pending
```

---

## Consuming Messages

### `consumer.consume(handler: MessageHandler): Promise<void>`

Starts consuming messages from the topic. Blocks until the `signal` (passed in `ConsumerOptions`) is aborted, then returns.

```typescript
const signal = AbortSignal.timeout(60_000)
await consumer.consume(async (msg) => {
  // Process the message.
  await processPayload(msg.payload)
  // Return normally to Ack; throw an error to Nack
})
```

**Ack/Nack behaviour:**

| Handler return | Effect |
|---|---|
| Returns normally | `Ack` — message is permanently removed from the queue |
| Throws an error | `Nack` — message re-appears after the visibility timeout; the error message is stored as the failure reason |

**Reconnection:** if the stream is interrupted (network error, server restart), `consume` reconnects automatically with exponential backoff starting at 500 ms, doubling up to a maximum of 30 s.

**Concurrency:** messages are dispatched to the handler up to `concurrency` times in parallel. The promise resolves after the handler completes and the Ack/Nack is recorded.

```typescript
import { signal } from 'os'

const abortController = new AbortController()

// Stop gracefully on SIGINT/SIGTERM
process.on('SIGINT', () => abortController.abort())
process.on('SIGTERM', () => abortController.abort())

const consumer = client.consumer('orders', {
  concurrency: 4,
  signal: abortController.signal,
})

await consumer.consume(async (msg) => {
  console.log(`processing ${msg.id}`)
  await processPayload(msg.payload)
})
```

### `consumer.consumeBatch(options: BatchOptions, handler: BatchHandler): Promise<void>`

Polls the queue in batches and dispatches each batch to the handler. Returns when the `signal` (passed in `ConsumerOptions`) is aborted.

```typescript
const signal = AbortSignal.timeout(60_000)
await consumer.consumeBatch(
  { batchSize: 50, visibilityTimeoutSeconds: 60 },
  async (messages) => {
    // Process all messages in the batch.
    for (const msg of messages) {
      try {
        await processPayload(msg.payload)
        await msg.ack()
      } catch (err) {
        await msg.nack(err instanceof Error ? err.message : 'unknown error')
      }
    }
  },
)
```

**Batch semantics:**

- `batchSize`: number of messages to request per poll (valid range 1–1000).
- **Best-effort:** returns 0–N messages per call, never blocks or waits for a full batch. When the queue is empty or throughput-limited, the consumer applies the same exponential backoff as `consume` (500 ms → 30 s). When messages are returned, backoff resets.
- **Per-message ack/nack:** each message in the array has individual `ack()` and `nack(reason)` methods. Call them directly to acknowledge or reject each message.
- **Reconnection & backoff:** network errors are retried with exponential backoff (500 ms → 30 s), same as `consume`.

Use `consumeBatch` when you want to process multiple messages together (e.g. batch writes to a data warehouse) or when you need more control over per-message error handling.

### ConsumerOptions

| Option | Type | Description |
|---|---|---|
| `concurrency?` | number | Number of messages processed concurrently (default: `1`) |
| `visibilityTimeoutSeconds?` | number | How long a message stays invisible while being processed (default: server setting, typically 30 s) |
| `consumerGroup?` | string | Consumer group name for independent message consumption; see [Consumer Groups](#consumer-groups) |
| `signal?` | `AbortSignal` | Signal to abort the consumer loop |

---

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

### Single-Consumer (Consume)

```typescript
const consumer = client.consumer('orders', {
  consumerGroup: 'warehouse',
  concurrency: 4,
})

await consumer.consume(async (msg) => {
  console.log(`[warehouse] processing ${msg.id}`)
  // return normally to Ack; throw to Nack
})
```

### Batch Consumer (ConsumeBatch)

```typescript
await consumer.consumeBatch(
  { batchSize: 50, consumerGroup: 'warehouse' },
  async (messages) => {
    for (const msg of messages) {
      try {
        await processPayload(msg.payload)
        await msg.ack()
      } catch (err) {
        await msg.nack(err instanceof Error ? err.message : 'unknown')
      }
    }
  },
)
```

---

## The Message Type

```typescript
interface Message {
  id: string
  topic: string
  payload: Buffer
  metadata: Record<string, string>
  createdAt: Date
  retryCount: number
  ack(): Promise<void>
  nack(reason: string): Promise<void>
}
```

- `id`: The assigned message ID.
- `topic`: The topic the message was enqueued on.
- `payload`: The message body (as a Buffer).
- `metadata`: Arbitrary key-value metadata attached at publish time.
- `createdAt`: The timestamp when the message was first enqueued.
- `retryCount`: The number of times this message has been nacked (0 on first receive).

### `msg.ack(): Promise<void>`

Permanently removes the message from the queue. When using `consume` with a handler, ack is called automatically on successful completion — call this directly only when using `consumeBatch` or managing ack/nack manually.

### `msg.nack(reason: string): Promise<void>`

Returns the message to the queue. `reason` is stored as the failure string and is visible in the Admin UI.

---

## Authentication

When the server has `auth.enabled = true`, every RPC call requires a valid JWT. Tokens are issued by the server's HTTP API and expire after 15 minutes.

### Obtaining a token

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}' \
  | jq -r '.token')
```

### Option 1 — Automatic refresh (recommended)

Pass an initial token and a `tokenRefresher` callback. The library decodes the JWT `exp` claim, sleeps until 60 seconds before expiry, and calls your callback to obtain a fresh token. The new token is applied to the next RPC call — no reconnection needed.

```typescript
async function fetchToken(): Promise<string> {
  const resp = await fetch('http://localhost:8080/api/auth/refresh', {
    method: 'POST',
    headers: { Authorization: `Bearer ${currentToken}` },
  })
  const { token } = await resp.json()
  return token
}

const client = await connect('localhost:50051', {
  insecure: true,
  token: initialToken,
  tokenRefresher: fetchToken,
})
```

If the refresher returns an error, the library retries with exponential backoff (5 s → 60 s) and logs each failure. RPCs will start failing with `Unauthenticated` once the token expires, so ensure the refresher can recover.

### Option 2 — Manual update

Call `client.setToken()` to swap the token on a live connection. The new token takes effect on the very next RPC call; no reconnection is needed.

```typescript
const client = await connect('localhost:50051', {
  insecure: true,
  token: initialToken,
})

// Later, when you have a fresh token:
client.setToken(newToken)
```

This is useful when token lifecycle is managed externally (e.g. a shared token store, a sidecar, or an existing refresh loop in your application).

### Option 3 — Static token (short-lived processes)

For scripts or jobs that complete within the 15-minute token window, a static token is sufficient:

```typescript
const client = await connect('localhost:50051', {
  insecure: true,
  token: process.env.QUEUETI_TOKEN!,
})
```

---

## Error Handling

`publish` wraps the underlying gRPC error and includes the topic name:

```typescript
try {
  const id = await producer.publish('orders', payload)
} catch (err) {
  // e.g. "publish to topic \"orders\": rpc error: code = Unauthenticated ..."
  console.error(err)
}
```

`consume` and `consumeBatch` only throw errors for programming mistakes (invalid configuration). Network errors and stream interruptions are handled internally via reconnection. A clean shutdown (signal aborted) always resolves normally.

When a stream error occurs, the library logs the error and reconnects with exponential backoff. Your handler is not called during network outages.

---

## Full Example

```typescript
import { connect } from '@queue-ti/client'

async function main() {
  const initialToken = process.env.QUEUETI_TOKEN || 'your-token-here'

  const client = await connect('localhost:50051', {
    insecure: true,
    token: initialToken,
    tokenRefresher: async () => {
      const resp = await fetch('http://localhost:8080/api/auth/refresh', {
        method: 'POST',
        headers: { Authorization: `Bearer ${initialToken}` },
      })
      const { token } = await resp.json()
      return token
    },
  })

  // Publish one message
  const producer = client.producer()
  const id = await producer.publish('orders', Buffer.from(JSON.stringify({ item: 'book' })), {
    metadata: { source: 'example' },
  })
  console.log('published:', id)

  // Consume until SIGINT
  const abortController = new AbortController()
  process.on('SIGINT', () => {
    console.log('shutting down...')
    abortController.abort()
  })

  const consumer = client.consumer('orders', {
    concurrency: 4,
    signal: abortController.signal,
  })

  await consumer.consume(async (msg) => {
    console.log(`[${msg.id}] ${msg.payload.toString()}`)
    // return normally to Ack
  })

  client.close()
  console.log('consumer stopped')
}

main().catch(console.error)
```

---

## TypeScript

The library ships with complete TypeScript type definitions. All public APIs are fully typed, including request/response shapes and options.

```typescript
import { connect, ConnectOptions, ConsumerOptions, PublishOptions, Message } from '@queue-ti/client'
```
