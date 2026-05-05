import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueueTiAuth } from '../auth'

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

beforeEach(() => {
  mockFetch.mockReset()
})

describe('QueueTiAuth', () => {
  describe('login()', () => {
    describe('when auth is not required', () => {
      it('returns an instance with null token without calling login', async () => {
        mockFetch.mockResolvedValueOnce(jsonResponse({ auth_required: false }))

        const auth = await QueueTiAuth.login('http://localhost:8080', 'user', 'pass')

        expect(auth.token).toBeNull()
        expect(mockFetch).toHaveBeenCalledTimes(1)
        expect(mockFetch.mock.calls[0][0]).toContain('/api/auth/status')
      })

      it('strips a trailing slash from adminAddr', async () => {
        mockFetch.mockResolvedValueOnce(jsonResponse({ auth_required: false }))

        await QueueTiAuth.login('http://localhost:8080/', 'u', 'p')

        expect(mockFetch.mock.calls[0][0]).toBe('http://localhost:8080/api/auth/status')
      })
    })

    describe('when auth is required', () => {
      it('returns an instance with the token from the login response', async () => {
        mockFetch
          .mockResolvedValueOnce(jsonResponse({ auth_required: true }))
          .mockResolvedValueOnce(jsonResponse({ token: 'jwt-abc' }))

        const auth = await QueueTiAuth.login('http://localhost:8080', 'admin', 'secret')

        expect(auth.token).toBe('jwt-abc')
        expect(mockFetch).toHaveBeenCalledTimes(2)
      })

      it('sends credentials as JSON in the login body', async () => {
        mockFetch
          .mockResolvedValueOnce(jsonResponse({ auth_required: true }))
          .mockResolvedValueOnce(jsonResponse({ token: 'tok' }))

        await QueueTiAuth.login('http://localhost:8080', 'my"user', 'p\\a"ss')

        const [url, init] = mockFetch.mock.calls[1]
        expect(url).toContain('/api/auth/login')
        const body = JSON.parse(init.body as string)
        expect(body.username).toBe('my"user')
        expect(body.password).toBe('p\\a"ss')
      })

      it('throws when the status endpoint returns a non-2xx response', async () => {
        mockFetch.mockResolvedValueOnce(new Response('Internal error', { status: 500 }))

        await expect(QueueTiAuth.login('http://localhost:8080', 'u', 'p')).rejects.toThrow(
          /check auth status.*500/,
        )
      })

      it('throws when login returns a non-2xx response', async () => {
        mockFetch
          .mockResolvedValueOnce(jsonResponse({ auth_required: true }))
          .mockResolvedValueOnce(new Response('Unauthorized', { status: 401 }))

        await expect(QueueTiAuth.login('http://localhost:8080', 'bad', 'creds')).rejects.toThrow(
          /login.*401/,
        )
      })

      it('throws when the login response has an empty token', async () => {
        mockFetch
          .mockResolvedValueOnce(jsonResponse({ auth_required: true }))
          .mockResolvedValueOnce(jsonResponse({ token: '' }))

        await expect(QueueTiAuth.login('http://localhost:8080', 'u', 'p')).rejects.toThrow(
          /empty token/,
        )
      })
    })
  })

  describe('refresh()', () => {
    it('returns empty string when auth is disabled', async () => {
      mockFetch.mockResolvedValueOnce(jsonResponse({ auth_required: false }))

      const auth = await QueueTiAuth.login('http://localhost:8080', 'u', 'p')
      const result = await auth.refresh()

      expect(result).toBe('')
      expect(mockFetch).toHaveBeenCalledTimes(1)
    })

    it('re-authenticates and returns the new token', async () => {
      mockFetch
        .mockResolvedValueOnce(jsonResponse({ auth_required: true }))
        .mockResolvedValueOnce(jsonResponse({ token: 'initial-token' }))
        .mockResolvedValueOnce(jsonResponse({ token: 'refreshed-token' }))

      const auth = await QueueTiAuth.login('http://localhost:8080', 'admin', 'secret')
      expect(auth.token).toBe('initial-token')

      const newToken = await auth.refresh()
      expect(newToken).toBe('refreshed-token')
      expect(auth.token).toBe('refreshed-token')
    })
  })
})
