export type TokenRefresher = () => Promise<string>

export interface TLSOptions {
  rootCerts?: Buffer      // PEM-encoded CA certificate(s); uses system CAs when omitted
  privateKey?: Buffer     // PEM-encoded client private key for mTLS; requires certChain
  certChain?: Buffer      // PEM-encoded client certificate chain for mTLS; requires privateKey
  serverNameOverride?: string  // override the hostname used for TLS SNI/verification
}

export interface ConnectOptions {
  insecure?: boolean
  tls?: TLSOptions        // custom TLS config; ignored when insecure is true
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
