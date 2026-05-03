import { describe, it, expect, vi } from 'vitest'
import { Consumer, ConsumerStub, RawMessage, SubscribeStream } from '../consumer'
import { Message } from '../message'
import { EventEmitter } from 'events'

// A fake stream that is also an EventEmitter so tests can push data/errors/end.
class FakeStream extends EventEmitter implements SubscribeStream {
  cancel = vi.fn()

  push(msg: RawMessage): void {
    this.emit('data', msg)
  }

  end(): void {
    this.emit('end')
  }

  error(err: Error): void {
    this.emit('error', err)
  }
}

function makeRawMessage(overrides?: Partial<RawMessage>): RawMessage {
  return {
    id: 'msg-1',
    topic: 'test-topic',
    payload: Buffer.from('hello'),
    metadata: {},
    createdAt: { seconds: 1700000000, nanos: 0 },
    retryCount: 0,
    ...overrides,
  }
}

function makeStub(stream: FakeStream, overrides?: Partial<ConsumerStub>): ConsumerStub {
  return {
    subscribe: vi.fn(() => stream),
    batchDequeue: vi.fn((_req, cb) => cb(null, { messages: [] })),
    ack: vi.fn((_req, cb) => cb(null)),
    nack: vi.fn((_req, cb) => cb(null)),
    ...overrides,
  }
}

// Yields to the microtask/macrotask queue so async code that was kicked off
// can run before we assert.
function tick(): Promise<void> {
  return new Promise<void>((resolve) => setImmediate(resolve))
}

describe('Consumer', () => {
  describe('consume()', () => {
    describe('when the handler returns without throwing', () => {
      it('should ack the message', async () => {
        const stream = new FakeStream()
        const stub = makeStub(stream)
        const controller = new AbortController()

        const ackReceived = new Promise<void>((resolve) => {
          (stub.ack as ReturnType<typeof vi.fn>).mockImplementation((_req: { id: string }, cb: (err: null) => void) => {
            cb(null)
            resolve()
          })
        })

        const consumer = new Consumer(stub, 'test-topic', { signal: controller.signal })
        const consumePromise = consumer.consume(async (_msg: Message) => {
          // handler returns normally → ack should fire
        })

        await tick() // let consume() call subscribe() and attach stream listeners

        stream.push(makeRawMessage())
        await ackReceived // wait until ack has actually been called

        controller.abort()
        stream.end()
        await consumePromise

        expect(stub.ack).toHaveBeenCalledOnce()
        expect(stub.ack).toHaveBeenCalledWith({ id: 'msg-1', consumerGroup: '' }, expect.any(Function))
      })

      it('should not call nack', async () => {
        const stream = new FakeStream()
        const stub = makeStub(stream)
        const controller = new AbortController()

        const ackReceived = new Promise<void>((resolve) => {
          (stub.ack as ReturnType<typeof vi.fn>).mockImplementation((_req: { id: string }, cb: (err: null) => void) => {
            cb(null)
            resolve()
          })
        })

        const consumer = new Consumer(stub, 'test-topic', { signal: controller.signal })
        const consumePromise = consumer.consume(async () => {})

        await tick()
        stream.push(makeRawMessage())
        await ackReceived

        controller.abort()
        stream.end()
        await consumePromise

        expect(stub.nack).not.toHaveBeenCalled()
      })
    })

    describe('when the handler throws', () => {
      it('should nack the message with the error reason', async () => {
        const stream = new FakeStream()
        const stub = makeStub(stream)
        const controller = new AbortController()

        const nackReceived = new Promise<void>((resolve) => {
          (stub.nack as ReturnType<typeof vi.fn>).mockImplementation((_req: unknown, cb: (err: null) => void) => {
            cb(null)
            resolve()
          })
        })

        const consumer = new Consumer(stub, 'test-topic', { signal: controller.signal })
        const consumePromise = consumer.consume(async () => {
          throw new Error('processing failed')
        })

        await tick()
        stream.push(makeRawMessage())
        await nackReceived

        controller.abort()
        stream.end()
        await consumePromise

        expect(stub.nack).toHaveBeenCalledOnce()
        expect(stub.nack).toHaveBeenCalledWith(
          { id: 'msg-1', error: 'processing failed', consumerGroup: '' },
          expect.any(Function),
        )
      })

      it('should not ack the message', async () => {
        const stream = new FakeStream()
        const stub = makeStub(stream)
        const controller = new AbortController()

        const nackReceived = new Promise<void>((resolve) => {
          (stub.nack as ReturnType<typeof vi.fn>).mockImplementation((_req: unknown, cb: (err: null) => void) => {
            cb(null)
            resolve()
          })
        })

        const consumer = new Consumer(stub, 'test-topic', { signal: controller.signal })
        const consumePromise = consumer.consume(async () => {
          throw new Error('boom')
        })

        await tick()
        stream.push(makeRawMessage())
        await nackReceived

        controller.abort()
        stream.end()
        await consumePromise

        expect(stub.ack).not.toHaveBeenCalled()
      })
    })

    describe('when signal is already aborted before consume()', () => {
      it('should return immediately without calling subscribe', async () => {
        const stream = new FakeStream()
        const stub = makeStub(stream)
        const consumer = new Consumer(stub, 'test-topic', { signal: AbortSignal.abort() })

        await consumer.consume(async () => {})

        expect(stub.subscribe).not.toHaveBeenCalled()
      })
    })

    describe('when the stream ends cleanly', () => {
      it('should reconnect and subscribe again', async () => {
        const stream1 = new FakeStream()
        const stream2 = new FakeStream()
        const controller = new AbortController()
        let callCount = 0

        const stub: ConsumerStub = {
          subscribe: vi.fn(() => {
            callCount++
            if (callCount === 1) return stream1
            return stream2
          }),
          batchDequeue: vi.fn((_req, cb) => cb(null, { messages: [] })),
          ack: vi.fn((_req, cb) => cb(null)),
          nack: vi.fn((_req, cb) => cb(null)),
        }

        const consumer = new Consumer(stub, 'test-topic', { signal: controller.signal })
        const consumePromise = consumer.consume(async () => {})

        await tick()        // let consume() attach listeners to stream1
        stream1.end()       // clean end → triggers reconnect

        await tick()        // let the reconnect loop call subscribe() again and attach to stream2
        await tick()

        controller.abort()  // now abort
        stream2.end()

        await consumePromise

        expect(stub.subscribe).toHaveBeenCalledTimes(2)
      })
    })
  })

  describe('consumeBatch()', () => {
    describe('when batchDequeue returns messages', () => {
      it('should call the handler with the batch', async () => {
        const stream = new FakeStream()
        const raw = makeRawMessage({ id: 'batch-msg-1' })
        const controller = new AbortController()

        const stub = makeStub(stream, {
          batchDequeue: vi.fn((_req, cb) => {
            cb(null, { messages: [raw] })
          }),
        })

        const consumer = new Consumer(stub, 'test-topic', { signal: controller.signal })

        const handler = vi.fn(async (_msgs: Message[]) => {
          controller.abort()
        })

        await consumer.consumeBatch({ batchSize: 5 }, handler)

        expect(handler).toHaveBeenCalledOnce()
        const [msgs] = handler.mock.calls[0] as [Message[]]
        expect(msgs).toHaveLength(1)
        expect(msgs[0].id).toBe('batch-msg-1')
      })
    })

    describe('when batchDequeue returns an empty batch', () => {
      it('should apply backoff and continue polling until aborted', async () => {
        const stream = new FakeStream()
        const controller = new AbortController()
        let calls = 0

        const stub = makeStub(stream, {
          batchDequeue: vi.fn((_req, cb) => {
            calls++
            if (calls >= 2) controller.abort()
            cb(null, { messages: [] })
          }),
        })

        const consumer = new Consumer(stub, 'test-topic', { signal: controller.signal })

        await consumer.consumeBatch({ batchSize: 10 }, async () => {})

        expect(calls).toBeGreaterThanOrEqual(1)
      })
    })
  })

  describe('when consumerGroup is set', () => {
    it('should send consumerGroup in subscribe request', async () => {
      const stream = new FakeStream()
      const stub = makeStub(stream)
      const controller = new AbortController()

      const consumer = new Consumer(stub, 'test-topic', {
        signal: controller.signal,
        consumerGroup: 'workers',
      })
      const consumePromise = consumer.consume(async () => {})

      await tick()
      controller.abort()
      stream.end()
      await consumePromise

      expect(stub.subscribe).toHaveBeenCalledWith(
        expect.objectContaining({ consumerGroup: 'workers' }),
      )
    })

    it('should carry consumerGroup in ack request', async () => {
      const stream = new FakeStream()
      const stub = makeStub(stream)
      const controller = new AbortController()

      const ackReceived = new Promise<void>((resolve) => {
        (stub.ack as ReturnType<typeof vi.fn>).mockImplementation(
          (_req: { id: string; consumerGroup: string }, cb: (err: null) => void) => {
            cb(null)
            resolve()
          },
        )
      })

      const consumer = new Consumer(stub, 'test-topic', {
        signal: controller.signal,
        consumerGroup: 'workers',
      })
      const consumePromise = consumer.consume(async () => {})

      await tick()
      stream.push(makeRawMessage())
      await ackReceived

      controller.abort()
      stream.end()
      await consumePromise

      expect(stub.ack).toHaveBeenCalledWith(
        { id: 'msg-1', consumerGroup: 'workers' },
        expect.any(Function),
      )
    })

    it('should carry consumerGroup in nack request', async () => {
      const stream = new FakeStream()
      const stub = makeStub(stream)
      const controller = new AbortController()

      const nackReceived = new Promise<void>((resolve) => {
        (stub.nack as ReturnType<typeof vi.fn>).mockImplementation(
          (_req: unknown, cb: (err: null) => void) => {
            cb(null)
            resolve()
          },
        )
      })

      const consumer = new Consumer(stub, 'test-topic', {
        signal: controller.signal,
        consumerGroup: 'workers',
      })
      const consumePromise = consumer.consume(async () => {
        throw new Error('fail')
      })

      await tick()
      stream.push(makeRawMessage())
      await nackReceived

      controller.abort()
      stream.end()
      await consumePromise

      expect(stub.nack).toHaveBeenCalledWith(
        { id: 'msg-1', error: 'fail', consumerGroup: 'workers' },
        expect.any(Function),
      )
    })

    it('should send consumerGroup in batchDequeue request', async () => {
      const stream = new FakeStream()
      const controller = new AbortController()

      const stub = makeStub(stream, {
        batchDequeue: vi.fn((_req, cb) => {
          controller.abort()
          cb(null, { messages: [] })
        }),
      })

      const consumer = new Consumer(stub, 'test-topic', {
        signal: controller.signal,
        consumerGroup: 'workers',
      })

      await consumer.consumeBatch({ batchSize: 5 }, async () => {})

      expect(stub.batchDequeue).toHaveBeenCalledWith(
        expect.objectContaining({ consumerGroup: 'workers' }),
        expect.any(Function),
      )
    })

    it('should carry consumerGroup in batch ack request', async () => {
      const stream = new FakeStream()
      const raw = makeRawMessage({ id: 'batch-msg-1' })
      const controller = new AbortController()

      const stub = makeStub(stream, {
        batchDequeue: vi.fn((_req, cb) => {
          cb(null, { messages: [raw] })
        }),
      })

      const consumer = new Consumer(stub, 'test-topic', {
        signal: controller.signal,
        consumerGroup: 'workers',
      })

      await consumer.consumeBatch({ batchSize: 5 }, async (msgs: Message[]) => {
        await msgs[0].ack()
        controller.abort()
      })

      expect(stub.ack).toHaveBeenCalledWith(
        { id: 'batch-msg-1', consumerGroup: 'workers' },
        expect.any(Function),
      )
    })
  })
})
