/**
 * Order pipeline — demonstrates the full producer → consumer → ack lifecycle
 * against a local queue-ti instance.
 *
 * Run: npx ts-node index.ts (requires docker-compose up)
 */

import { connect, AdminClient, QueueTiAuth } from '../../src'
import type { Message } from '../../src/message'
import type { MessageHandler } from '../../src/consumer'

const GRPC_ADDR = 'localhost:50051'
const ADMIN_URL = 'http://localhost:8080'
const TOPIC = 'orders'
const DLQ_TOPIC = 'orders.dlq'
const CONSUMER_GROUP = 'fulfillment'
const USERNAME = process.env.QUEUETI_USERNAME ?? 'admin'
const PASSWORD = process.env.QUEUETI_PASSWORD ?? 'secret'

interface Order {
  id: string
  item: string
  amount: number
  poison?: boolean
}

const orders: Order[] = [
  { id: 'ord-1', item: 'Widget A', amount: 2 },
  { id: 'ord-2', item: 'Gadget B', amount: 1 },
  { id: 'ord-3', item: 'poison', amount: 0, poison: true },
  { id: 'ord-4', item: 'Widget C', amount: 5 },
  { id: 'ord-5', item: 'Gadget D', amount: 3 },
]

async function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function registerConsumerGroup(auth: QueueTiAuth): Promise<void> {
  const admin = new AdminClient(ADMIN_URL, { token: auth.token ?? undefined })
  try {
    await admin.registerConsumerGroup(TOPIC, CONSUMER_GROUP)
    console.log(`consumer group "${CONSUMER_GROUP}" registered`)
  } catch (err: unknown) {
    // 409 Conflict means the group already exists — that's fine
    if (err instanceof Error && err.message.includes('409')) {
      console.log(`consumer group "${CONSUMER_GROUP}" already exists`)
    } else {
      throw err
    }
  }
}

async function produce(auth: QueueTiAuth, controller: AbortController): Promise<void> {
  const client = await connect(GRPC_ADDR, {
    insecure: true,
    token: auth.token ?? undefined,
    tokenRefresher: auth.refresh,
  })
  const producer = client.producer()

  for (const order of orders) {
    if (controller.signal.aborted) break
    await sleep(500)
    const payload = Buffer.from(JSON.stringify(order))
    const id = await producer.publish(TOPIC, payload, {
      metadata: { source: 'order-pipeline' },
      key: order.id,
    })
    console.log(`published ${order.id} → message ${id}`)
  }

  client.close()
}

async function consume(auth: QueueTiAuth, controller: AbortController): Promise<void> {
  const client = await connect(GRPC_ADDR, {
    insecure: true,
    token: auth.token ?? undefined,
    tokenRefresher: auth.refresh,
  })
  const consumer = client.consumer(TOPIC, {
    consumerGroup: CONSUMER_GROUP,
    concurrency: 3,
    signal: controller.signal,
  })

  console.log(`consuming from "${TOPIC}" (group "${CONSUMER_GROUP}") — Ctrl-C to stop`)

  const handler: MessageHandler = async (msg: Message) => {
    const order: Order = JSON.parse(msg.payload.toString())

    if (order.poison) {
      console.log(`nack ${msg.id}: poison pill detected (retry ${msg.retryCount})`)
      await msg.nack('poison pill')
      return
    }

    console.log(`ack ${msg.id}: processed order ${order.id} — ${order.amount}×${order.item}`)
    await msg.ack()
  }

  await consumer.consume(handler)
  client.close()
}

async function drainDLQ(auth: QueueTiAuth): Promise<void> {
  const client = await connect(GRPC_ADDR, {
    insecure: true,
    token: auth.token ?? undefined,
    tokenRefresher: auth.refresh,
  })
  const consumer = client.consumer(DLQ_TOPIC, { consumerGroup: CONSUMER_GROUP })

  console.log(`draining DLQ "${DLQ_TOPIC}"`)

  await consumer.consumeBatch(
    { batchSize: 10, consumerGroup: CONSUMER_GROUP },
    async (messages: Message[]) => {
      for (const msg of messages) {
        console.log(`[DLQ] ${msg.id} retry=${msg.retryCount} payload=${msg.payload.toString()}`)
        await msg.ack()
      }
    },
  )

  client.close()
}

async function main(): Promise<void> {
  const auth = await QueueTiAuth.login(ADMIN_URL, USERNAME, PASSWORD)

  const controller = new AbortController()

  process.on('SIGINT', () => {
    console.log('\nshutting down…')
    controller.abort()
  })
  process.on('SIGTERM', () => controller.abort())

  await registerConsumerGroup(auth)

  void produce(auth, controller)
  void drainDLQ(auth)
  await consume(auth, controller)
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
