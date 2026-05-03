export type TokenRefresher = () => Promise<string>

export interface ConnectOptions {
  insecure?: boolean
  token?: string
  tokenRefresher?: TokenRefresher
}

export interface PublishOptions {
  metadata?: Record<string, string>
  key?: string
}

export interface ConsumerOptions {
  concurrency?: number
  visibilityTimeoutSeconds?: number
  signal?: AbortSignal
  consumerGroup?: string
}

export interface BatchOptions {
  batchSize: number
  visibilityTimeoutSeconds?: number
  consumerGroup?: string
}
