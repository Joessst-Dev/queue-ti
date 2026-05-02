import { describe, it, expect } from 'vitest'
import { TokenStore, parseTokenExpiry } from '../token-store'

function makeJwt(payload: Record<string, unknown>): string {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const body = Buffer.from(JSON.stringify(payload)).toString('base64url')
  return `${header}.${body}.fakesig`
}

describe('TokenStore', () => {
  describe('when constructed with an initial token', () => {
    it('should return the initial token from get()', () => {
      const store = new TokenStore('initial-token')
      expect(store.get()).toBe('initial-token')
    })
  })

  describe('when set() is called', () => {
    it('should return the updated token from get()', () => {
      const store = new TokenStore('old')
      store.set('new')
      expect(store.get()).toBe('new')
    })
  })
})

describe('parseTokenExpiry', () => {
  describe('when given a valid JWT with a future exp', () => {
    it('should return the correct expiry date', () => {
      const exp = Math.floor(Date.now() / 1000) + 3600
      const token = makeJwt({ sub: 'user', exp })
      const result = parseTokenExpiry(token)
      expect(result).toBeInstanceOf(Date)
      expect(result.getTime()).toBe(exp * 1000)
    })
  })

  describe('when given a valid JWT with a past exp', () => {
    it('should return a date in the past', () => {
      const exp = Math.floor(Date.now() / 1000) - 60
      const token = makeJwt({ sub: 'user', exp })
      const result = parseTokenExpiry(token)
      expect(result.getTime()).toBeLessThan(Date.now())
    })
  })

  describe('when given a JWT with no exp claim', () => {
    it('should throw an error', () => {
      const token = makeJwt({ sub: 'user' })
      expect(() => parseTokenExpiry(token)).toThrow('JWT has no exp claim')
    })
  })

  describe('when given a malformed string with wrong segment count', () => {
    it('should throw a malformed JWT error', () => {
      expect(() => parseTokenExpiry('not.a.valid.jwt.here')).toThrow('malformed JWT')
    })
  })

  describe('when given a string with only one segment', () => {
    it('should throw a malformed JWT error', () => {
      expect(() => parseTokenExpiry('onlyone')).toThrow('malformed JWT')
    })
  })

  describe('when the payload segment is not valid base64', () => {
    it('should throw a decode error', () => {
      expect(() => parseTokenExpiry('header.!!!notbase64!!!.sig')).toThrow()
    })
  })
})
