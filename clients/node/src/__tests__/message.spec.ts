import { describe, it, expect, vi } from 'vitest'
import { buildMessage } from '../message'

function makeStubs() {
  const ackFn = vi.fn(() => Promise.resolve())
  const nackFn = vi.fn(() => Promise.resolve())
  return { ackFn, nackFn }
}

describe('buildMessage', () => {
  describe('when constructing a message', () => {
    it('should expose all fields correctly', () => {
      const { ackFn, nackFn } = makeStubs()
      const createdAt = new Date('2025-01-01T00:00:00Z')
      const msg = buildMessage(
        'id-1',
        'topic-a',
        Buffer.from('payload'),
        { key: 'val' },
        createdAt,
        2,
        ackFn,
        nackFn,
      )

      expect(msg.id).toBe('id-1')
      expect(msg.topic).toBe('topic-a')
      expect(msg.payload).toEqual(Buffer.from('payload'))
      expect(msg.metadata).toEqual({ key: 'val' })
      expect(msg.createdAt).toBe(createdAt)
      expect(msg.retryCount).toBe(2)
    })
  })

  describe('ack()', () => {
    it('should call ackFn with the message id', async () => {
      const { ackFn, nackFn } = makeStubs()
      const msg = buildMessage('msg-42', 'topic', Buffer.from(''), {}, new Date(), 0, ackFn, nackFn)

      await msg.ack()

      expect(ackFn).toHaveBeenCalledOnce()
      expect(ackFn).toHaveBeenCalledWith('msg-42')
    })

    it('should not call nackFn', async () => {
      const { ackFn, nackFn } = makeStubs()
      const msg = buildMessage('msg-42', 'topic', Buffer.from(''), {}, new Date(), 0, ackFn, nackFn)

      await msg.ack()

      expect(nackFn).not.toHaveBeenCalled()
    })
  })

  describe('nack()', () => {
    it('should call nackFn with the message id and reason', async () => {
      const { ackFn, nackFn } = makeStubs()
      const msg = buildMessage('msg-99', 'topic', Buffer.from(''), {}, new Date(), 0, ackFn, nackFn)

      await msg.nack('processing failed')

      expect(nackFn).toHaveBeenCalledOnce()
      expect(nackFn).toHaveBeenCalledWith('msg-99', 'processing failed')
    })

    it('should not call ackFn', async () => {
      const { ackFn, nackFn } = makeStubs()
      const msg = buildMessage('msg-99', 'topic', Buffer.from(''), {}, new Date(), 0, ackFn, nackFn)

      await msg.nack('oops')

      expect(ackFn).not.toHaveBeenCalled()
    })
  })
})
