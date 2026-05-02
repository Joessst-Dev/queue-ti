export class TokenStore {
  private token: string

  constructor(initial: string) {
    this.token = initial
  }

  get(): string {
    return this.token
  }

  set(token: string): void {
    this.token = token
  }
}

export function parseTokenExpiry(token: string): Date {
  const parts = token.split('.')
  if (parts.length !== 3) {
    throw new Error(`malformed JWT: expected 3 segments, got ${parts.length}`)
  }

  // base64url → base64 standard padding
  const segment = parts[1].replace(/-/g, '+').replace(/_/g, '/')
  const padded = segment + '='.repeat((4 - (segment.length % 4)) % 4)

  let payload: Record<string, unknown>
  try {
    payload = JSON.parse(Buffer.from(padded, 'base64').toString('utf8')) as Record<string, unknown>
  } catch {
    throw new Error('decode JWT payload: invalid JSON')
  }

  const exp = payload['exp']
  if (typeof exp !== 'number' || exp === 0) {
    throw new Error('JWT has no exp claim')
  }

  return new Date(exp * 1000)
}
