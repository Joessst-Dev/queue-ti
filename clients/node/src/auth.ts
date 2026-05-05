import type { TokenRefresher } from './options'

export class QueueTiAuth {
  readonly token: string | null
  private readonly adminAddr: string
  private readonly username: string
  private readonly password: string

  private constructor(
    adminAddr: string,
    username: string,
    password: string,
    token: string | null,
  ) {
    this.adminAddr = adminAddr
    this.username = username
    this.password = password
    this.token = token
  }

  /**
   * Checks whether auth is required on the server, and — when it is —
   * performs a login with username/password to obtain a JWT.
   *
   * @param adminAddr Base URL of the admin API, e.g. "http://localhost:8080".
   *                  A trailing slash is stripped automatically.
   */
  static async login(adminAddr: string, username: string, password: string): Promise<QueueTiAuth> {
    const base = adminAddr.replace(/\/+$/, '')

    const statusRes = await fetch(`${base}/api/auth/status`)
    if (!statusRes.ok) {
      throw new Error(`queue-ti auth: check auth status: HTTP ${statusRes.status}`)
    }
    const status = (await statusRes.json()) as { auth_required: boolean }

    if (!status.auth_required) {
      return new QueueTiAuth(base, username, password, null)
    }

    const token = await QueueTiAuth._doLogin(base, username, password)
    return new QueueTiAuth(base, username, password, token)
  }

  /**
   * Implements the TokenRefresher interface. Re-authenticates with the server
   * and returns the new token. When auth is disabled, returns an empty string.
   */
  readonly refresh: TokenRefresher = async (): Promise<string> => {
    if (this.token === null) {
      return ''
    }
    return QueueTiAuth._doLogin(this.adminAddr, this.username, this.password)
  }

  private static async _doLogin(base: string, username: string, password: string): Promise<string> {
    const res = await fetch(`${base}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`queue-ti auth: login: HTTP ${res.status}: ${text}`)
    }
    const body = (await res.json()) as { token: string }
    if (!body.token) {
      throw new Error('queue-ti auth: login: server returned empty token')
    }
    return body.token
  }
}
