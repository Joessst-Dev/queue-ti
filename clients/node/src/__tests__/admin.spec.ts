import { describe, it, expect, vi, beforeEach } from 'vitest'
import { AdminClient, AdminError } from '../admin'

const BASE_URL = 'http://localhost:8080'

function makeFetch(status: number, body: unknown): ReturnType<typeof vi.fn> {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: 'OK',
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(''),
  })
}

function makeClient(token?: string): AdminClient {
  return new AdminClient(BASE_URL, token ? { token } : undefined)
}

describe('AdminClient', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })

  describe('listTopicConfigs()', () => {
    describe('when the server returns a list of configs', () => {
      it('should return the items array', async () => {
        const items = [{ topic: 'orders', replayable: false }]
        vi.stubGlobal('fetch', makeFetch(200, { items }))

        const client = makeClient()
        const result = await client.listTopicConfigs()

        expect(result).toEqual(items)
      })

      it('should send a GET request to /api/topic-configs', async () => {
        const mockFetch = makeFetch(200, { items: [] })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().listTopicConfigs()

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topic-configs`,
          expect.objectContaining({ method: 'GET' }),
        )
      })

      it('should include the Authorization header when a token is set', async () => {
        const mockFetch = makeFetch(200, { items: [] })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient('secret-token').listTopicConfigs()

        expect(mockFetch).toHaveBeenCalledWith(
          expect.any(String),
          expect.objectContaining({
            headers: expect.objectContaining({ Authorization: 'Bearer secret-token' }),
          }),
        )
      })
    })
  })

  describe('upsertTopicConfig()', () => {
    describe('when the server accepts the config', () => {
      it('should send a PUT request with the JSON body', async () => {
        const returned = { topic: 'orders', replayable: true, max_retries: 3 }
        const mockFetch = makeFetch(200, returned)
        vi.stubGlobal('fetch', mockFetch)

        const input = { replayable: true, max_retries: 3 }
        const result = await makeClient().upsertTopicConfig('orders', input)

        expect(result).toEqual(returned)
        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topic-configs/orders`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify(input),
            headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
          }),
        )
      })
    })
  })

  describe('deleteTopicConfig()', () => {
    describe('when the server responds with 204', () => {
      it('should resolve void', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
          ok: true,
          status: 204,
          statusText: 'No Content',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve(''),
        }))

        await expect(makeClient().deleteTopicConfig('orders')).resolves.toBeUndefined()
      })

      it('should send a DELETE request to the correct path', async () => {
        const mockFetch = vi.fn().mockResolvedValue({
          ok: true,
          status: 204,
          statusText: 'No Content',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve(''),
        })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().deleteTopicConfig('orders')

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topic-configs/orders`,
          expect.objectContaining({ method: 'DELETE' }),
        )
      })
    })
  })

  describe('listTopicSchemas()', () => {
    describe('when the server returns schemas', () => {
      it('should return the items array', async () => {
        const items = [{ topic: 'orders', schema_json: '{}', version: 1, updated_at: '2024-01-01T00:00:00Z' }]
        vi.stubGlobal('fetch', makeFetch(200, { items }))

        const result = await makeClient().listTopicSchemas()

        expect(result).toEqual(items)
      })

      it('should send GET /api/topic-schemas', async () => {
        const mockFetch = makeFetch(200, { items: [] })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().listTopicSchemas()

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topic-schemas`,
          expect.objectContaining({ method: 'GET' }),
        )
      })
    })
  })

  describe('getTopicSchema()', () => {
    describe('when the schema exists', () => {
      it('should return the schema', async () => {
        const schema = { topic: 'orders', schema_json: '{}', version: 1, updated_at: '2024-01-01T00:00:00Z' }
        vi.stubGlobal('fetch', makeFetch(200, schema))

        const result = await makeClient().getTopicSchema('orders')

        expect(result).toEqual(schema)
      })
    })

    describe('when the server returns 404', () => {
      it('should throw AdminError with statusCode 404', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
          ok: false,
          status: 404,
          statusText: 'Not Found',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve('not found'),
        }))

        await expect(makeClient().getTopicSchema('missing')).rejects.toSatisfy(
          (err: unknown) => err instanceof AdminError && err.statusCode === 404,
        )
      })
    })
  })

  describe('upsertTopicSchema()', () => {
    describe('when the server accepts the schema', () => {
      it('should send schema_json in the request body', async () => {
        const returned = { topic: 'orders', schema_json: '{"type":"record"}', version: 2, updated_at: '2024-01-01T00:00:00Z' }
        const mockFetch = makeFetch(200, returned)
        vi.stubGlobal('fetch', mockFetch)

        const result = await makeClient().upsertTopicSchema('orders', '{"type":"record"}')

        expect(result).toEqual(returned)
        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topic-schemas/orders`,
          expect.objectContaining({
            method: 'PUT',
            body: JSON.stringify({ schema_json: '{"type":"record"}' }),
          }),
        )
      })
    })
  })

  describe('deleteTopicSchema()', () => {
    describe('when the server responds with 204', () => {
      it('should resolve void', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
          ok: true,
          status: 204,
          statusText: 'No Content',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve(''),
        }))

        await expect(makeClient().deleteTopicSchema('orders')).resolves.toBeUndefined()
      })

      it('should send DELETE /api/topic-schemas/{topic}', async () => {
        const mockFetch = vi.fn().mockResolvedValue({
          ok: true,
          status: 204,
          statusText: 'No Content',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve(''),
        })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().deleteTopicSchema('orders')

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topic-schemas/orders`,
          expect.objectContaining({ method: 'DELETE' }),
        )
      })
    })
  })

  describe('listConsumerGroups()', () => {
    describe('when the server returns a list of groups', () => {
      it('should return the items array', async () => {
        const items = ['group-a', 'group-b']
        vi.stubGlobal('fetch', makeFetch(200, { items }))

        const result = await makeClient().listConsumerGroups('orders')

        expect(result).toEqual(items)
      })

      it('should call the correct endpoint for the topic', async () => {
        const mockFetch = makeFetch(200, { items: [] })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().listConsumerGroups('orders')

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topics/orders/consumer-groups`,
          expect.objectContaining({ method: 'GET' }),
        )
      })
    })
  })

  describe('registerConsumerGroup()', () => {
    describe('when the server accepts the registration', () => {
      it('should send consumer_group in the request body', async () => {
        const mockFetch = vi.fn().mockResolvedValue({
          ok: true,
          status: 204,
          statusText: 'No Content',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve(''),
        })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().registerConsumerGroup('orders', 'billing')

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topics/orders/consumer-groups`,
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ consumer_group: 'billing' }),
          }),
        )
      })
    })

    describe('when the server returns 409', () => {
      it('should throw AdminError with statusCode 409', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
          ok: false,
          status: 409,
          statusText: 'Conflict',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve('consumer group already exists'),
        }))

        await expect(makeClient().registerConsumerGroup('orders', 'billing')).rejects.toSatisfy(
          (err: unknown) => err instanceof AdminError && err.statusCode === 409,
        )
      })
    })
  })

  describe('unregisterConsumerGroup()', () => {
    describe('when the server responds with 204', () => {
      it('should resolve void', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
          ok: true,
          status: 204,
          statusText: 'No Content',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve(''),
        }))

        await expect(makeClient().unregisterConsumerGroup('orders', 'billing')).resolves.toBeUndefined()
      })

      it('should send a DELETE request to the correct path', async () => {
        const mockFetch = vi.fn().mockResolvedValue({
          ok: true,
          status: 204,
          statusText: 'No Content',
          json: () => Promise.resolve(null),
          text: () => Promise.resolve(''),
        })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().unregisterConsumerGroup('orders', 'billing')

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/topics/orders/consumer-groups/billing`,
          expect.objectContaining({ method: 'DELETE' }),
        )
      })
    })
  })

  describe('stats()', () => {
    describe('when the server returns stats', () => {
      it('should return the topics array', async () => {
        const topics = [
          { topic: 'orders', status: 'pending', count: 42 },
          { topic: 'orders', status: 'acked', count: 100 },
        ]
        vi.stubGlobal('fetch', makeFetch(200, { topics }))

        const result = await makeClient().stats()

        expect(result).toEqual(topics)
      })

      it('should call GET /api/stats', async () => {
        const mockFetch = makeFetch(200, { topics: [] })
        vi.stubGlobal('fetch', mockFetch)

        await makeClient().stats()

        expect(mockFetch).toHaveBeenCalledWith(
          `${BASE_URL}/api/stats`,
          expect.objectContaining({ method: 'GET' }),
        )
      })
    })
  })

  describe('AdminError', () => {
    describe('when constructed with a status code and message', () => {
      it('should expose statusCode and format the message', () => {
        const err = new AdminError(404, 'not found')

        expect(err.statusCode).toBe(404)
        expect(err.message).toBe('admin: HTTP 404: not found')
        expect(err.name).toBe('AdminError')
        expect(err).toBeInstanceOf(Error)
      })
    })
  })
})
