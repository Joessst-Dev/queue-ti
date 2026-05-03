import { ConsumerOptions, BatchOptions } from './options'
import { Message, buildMessage } from './message'
import { sleep } from './internal/sleep'

export type MessageHandler = (msg: Message) => Promise<void>
export type BatchHandler = (messages: Message[]) => Promise<void>

const BACKOFF_START_MS = 500
const BACKOFF_MAX_MS = 30_000

function nextBackoff(current: number): number {
  return Math.min(current * 2, BACKOFF_MAX_MS)
}

function callbackToPromise<T>(
  fn: (cb: (err: Error | null, res: T) => void) => void,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    fn((err, res) => {
      if (err) reject(err)
      else resolve(res)
    })
  })
}

// ProtoTimestamp mirrors the shape @grpc/proto-loader produces for google.protobuf.Timestamp.
interface ProtoTimestamp {
  seconds: string | number
  nanos: number
}

function protoTimestampToDate(ts: ProtoTimestamp | null | undefined): Date {
  if (!ts) return new Date(0)
  const secs = typeof ts.seconds === 'string' ? parseInt(ts.seconds, 10) : ts.seconds
  return new Date(secs * 1000 + Math.floor(ts.nanos / 1_000_000))
}

export interface RawMessage {
  id: string
  topic: string
  payload: Buffer
  metadata: Record<string, string>
  createdAt: ProtoTimestamp | null | undefined
  retryCount: number
}

export interface SubscribeStream {
  on(event: 'data', handler: (msg: RawMessage) => void): this
  on(event: 'error', handler: (err: Error) => void): this
  on(event: 'end', handler: () => void): this
  cancel(): void
}

export interface ConsumerStub {
  subscribe(request: {
    topic: string
    visibilityTimeoutSeconds?: number
    consumerGroup?: string
  }): SubscribeStream

  batchDequeue(
    request: { topic: string; count: number; visibilityTimeoutSeconds?: number; consumerGroup?: string },
    callback: (err: Error | null, response: { messages: RawMessage[] }) => void,
  ): void

  ack(request: { id: string; consumerGroup?: string }, callback: (err: Error | null) => void): void
  nack(request: { id: string; error: string; consumerGroup?: string }, callback: (err: Error | null) => void): void
}

export class Consumer {
  private readonly topic: string
  private readonly concurrency: number
  private readonly visibilityTimeoutSeconds: number | undefined
  private readonly signal: AbortSignal | undefined
  private readonly consumerGroup: string

  constructor(
    private readonly stub: ConsumerStub,
    topic: string,
    options?: ConsumerOptions,
  ) {
    this.topic = topic
    this.concurrency = options?.concurrency ?? 1
    this.visibilityTimeoutSeconds = options?.visibilityTimeoutSeconds
    this.signal = options?.signal
    this.consumerGroup = options?.consumerGroup ?? ''
  }

  async consume(handler: MessageHandler): Promise<void> {
    let backoff = BACKOFF_START_MS

    while (!this.signal?.aborted) {
      const req: { topic: string; visibilityTimeoutSeconds?: number; consumerGroup: string } = {
        topic: this.topic,
        consumerGroup: this.consumerGroup,
      }
      if (this.visibilityTimeoutSeconds !== undefined) {
        req.visibilityTimeoutSeconds = this.visibilityTimeoutSeconds
      }

      let stream: SubscribeStream
      try {
        stream = this.stub.subscribe(req)
      } catch (err) {
        if (this.signal?.aborted) return
        console.error(`queue-ti consumer: subscribe error (retrying in ${backoff}ms):`, err)
        try {
          await sleep(backoff, this.signal)
        } catch {
          return
        }
        backoff = nextBackoff(backoff)
        continue
      }

      const cleanExit = await this.drainStream(stream, handler)

      if (cleanExit) {
        backoff = BACKOFF_START_MS
      }

      if (this.signal?.aborted) return

      if (!cleanExit) {
        try {
          await sleep(backoff, this.signal)
        } catch {
          return
        }
        backoff = nextBackoff(backoff)
      }
    }
  }

  private drainStream(
    stream: SubscribeStream,
    handler: MessageHandler,
  ): Promise<boolean> {
    // Semaphore: track active handler slots via a counter + a queue of resolvers
    // waiting for a free slot.
    let activeCount = 0
    const waitQueue: Array<() => void> = []

    const acquireSlot = (): Promise<void> => {
      if (activeCount < this.concurrency) {
        activeCount++
        return Promise.resolve()
      }
      return new Promise<void>((resolve) => waitQueue.push(resolve))
    }

    const releaseSlot = (): void => {
      const next = waitQueue.shift()
      if (next) {
        next()
      } else {
        activeCount--
      }
    }

    return new Promise<boolean>((resolve) => {
      let settled = false
      // Track all in-flight handler promises so we can await them before resolving.
      const inFlight: Promise<void>[] = []

      const settle = (cleanExit: boolean): void => {
        if (settled) return
        settled = true
        // Wait for all in-flight handlers before resolving so we don't drop messages.
        void Promise.allSettled(inFlight).then(() => resolve(cleanExit))
      }

      if (this.signal) {
        this.signal.addEventListener('abort', () => {
          stream.cancel()
          settle(true)
        }, { once: true })
      }

      stream.on('error', (err) => {
        if (this.signal?.aborted) {
          settle(true)
          return
        }
        console.error('queue-ti consumer: stream error (will reconnect):', err)
        settle(false)
      })

      stream.on('end', () => settle(true))

      stream.on('data', (raw: RawMessage) => {
        const msg = this.rawToMessage(raw)

        const handlerPromise = acquireSlot().then(async () => {
          try {
            await this.dispatch(msg, handler)
          } finally {
            releaseSlot()
          }
        })

        inFlight.push(handlerPromise)
      })
    })
  }

  private async dispatch(msg: Message, handler: MessageHandler): Promise<void> {
    let threw = false
    let thrownReason = 'unknown error'
    try {
      await handler(msg)
    } catch (err) {
      threw = true
      thrownReason = err instanceof Error ? err.message : String(err)
    }

    if (!threw) {
      try {
        await msg.ack()
      } catch (err) {
        if (!this.signal?.aborted) {
          console.error(`queue-ti consumer: ack failed for message ${msg.id}:`, err)
        }
      }
      return
    }

    try {
      await msg.nack(thrownReason)
    } catch (err) {
      if (!this.signal?.aborted) {
        console.error(`queue-ti consumer: nack failed for message ${msg.id}:`, err)
      }
    }
  }

  async consumeBatch(options: BatchOptions, handler: BatchHandler): Promise<void> {
    let backoff = BACKOFF_START_MS

    while (!this.signal?.aborted) {
      const req: { topic: string; count: number; visibilityTimeoutSeconds?: number; consumerGroup: string } = {
        topic: this.topic,
        count: options.batchSize,
        consumerGroup: this.consumerGroup,
      }
      if (options.visibilityTimeoutSeconds !== undefined) {
        req.visibilityTimeoutSeconds = options.visibilityTimeoutSeconds
      }

      let messages: RawMessage[]
      try {
        messages = await this.batchDequeue(req)
      } catch (err) {
        if (this.signal?.aborted) return
        console.error(`queue-ti consumer: batchDequeue error (retrying in ${backoff}ms):`, err)
        try {
          await sleep(backoff, this.signal)
        } catch {
          return
        }
        backoff = nextBackoff(backoff)
        continue
      }

      if (messages.length === 0) {
        try {
          await sleep(backoff, this.signal)
        } catch {
          return
        }
        backoff = nextBackoff(backoff)
        continue
      }

      backoff = BACKOFF_START_MS

      const batch = messages.map((raw) => this.rawToMessage(raw))
      try {
        await handler(batch)
      } catch (err) {
        if (!this.signal?.aborted) {
          console.error('queue-ti consumer: batch handler error:', err)
        }
      }
    }
  }

  private batchDequeue(req: {
    topic: string
    count: number
    visibilityTimeoutSeconds?: number
  }): Promise<RawMessage[]> {
    return callbackToPromise<{ messages: RawMessage[] }>(
      (cb) => this.stub.batchDequeue(req, cb),
    ).then((response) => response.messages ?? [])
  }

  private rawToMessage(raw: RawMessage): Message {
    const ackFn = (id: string): Promise<void> =>
      callbackToPromise<void>((cb) => this.stub.ack({ id, consumerGroup: this.consumerGroup }, cb))
        .catch((err: Error) => { throw new Error(`ack message ${id}: ${err.message}`) })

    const nackFn = (id: string, reason: string): Promise<void> =>
      callbackToPromise<void>((cb) => this.stub.nack({ id, error: reason, consumerGroup: this.consumerGroup }, cb))
        .catch((err: Error) => { throw new Error(`nack message ${id}: ${err.message}`) })

    return buildMessage(
      raw.id,
      raw.topic,
      Buffer.isBuffer(raw.payload) ? raw.payload : Buffer.from(raw.payload),
      raw.metadata ?? {},
      protoTimestampToDate(raw.createdAt),
      raw.retryCount ?? 0,
      ackFn,
      nackFn,
    )
  }
}

