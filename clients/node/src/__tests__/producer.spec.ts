import { describe, it, expect, vi } from 'vitest'
import { Producer, ProducerStub } from '../producer'

function makeStub(overrides?: Partial<ProducerStub>): ProducerStub {
  return {
    enqueue: vi.fn((_req, cb) => cb(null, { id: 'msg-123' })),
    ...overrides,
  }
}

describe('Producer', () => {
  describe('publish()', () => {
    describe('when the gRPC call succeeds', () => {
      it('should call Enqueue with the correct topic and payload', async () => {
        const stub = makeStub()
        const producer = new Producer(stub)

        await producer.publish('my-topic', Buffer.from('hello'))

        expect(stub.enqueue).toHaveBeenCalledWith(
          expect.objectContaining({ topic: 'my-topic', payload: Buffer.from('hello') }),
          expect.any(Function),
        )
      })

      it('should return the message ID from the response', async () => {
        const stub = makeStub()
        const producer = new Producer(stub)

        const id = await producer.publish('my-topic', Buffer.from('hello'))

        expect(id).toBe('msg-123')
      })

      it('should include metadata in the request when provided', async () => {
        const stub = makeStub()
        const producer = new Producer(stub)

        await producer.publish('topic', Buffer.from('data'), { metadata: { env: 'prod' } })

        expect(stub.enqueue).toHaveBeenCalledWith(
          expect.objectContaining({ metadata: { env: 'prod' } }),
          expect.any(Function),
        )
      })

      it('should include key in the request when provided', async () => {
        const stub = makeStub()
        const producer = new Producer(stub)

        await producer.publish('topic', Buffer.from('data'), { key: 'dedup-key' })

        expect(stub.enqueue).toHaveBeenCalledWith(
          expect.objectContaining({ key: 'dedup-key' }),
          expect.any(Function),
        )
      })

      it('should accept Uint8Array payload and convert to Buffer', async () => {
        const stub = makeStub()
        const producer = new Producer(stub)

        await producer.publish('topic', new Uint8Array([1, 2, 3]))

        const call = (stub.enqueue as ReturnType<typeof vi.fn>).mock.calls[0][0] as { payload: Buffer }
        expect(Buffer.isBuffer(call.payload)).toBe(true)
      })
    })

    describe('when the gRPC call fails', () => {
      it('should reject with an error that includes the topic name', async () => {
        const stub = makeStub({
          enqueue: vi.fn((_req, cb) => cb(new Error('unavailable'), { id: '' })),
        })
        const producer = new Producer(stub)

        await expect(producer.publish('my-topic', Buffer.from('x'))).rejects.toThrow(
          'publish to topic "my-topic"',
        )
      })
    })
  })
})
