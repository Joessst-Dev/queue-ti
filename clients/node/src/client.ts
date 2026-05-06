import * as protoLoader from '@grpc/proto-loader'
import * as grpc from '@grpc/grpc-js'
import fs from 'fs'
import path from 'path'
import { ConnectOptions, ConsumerOptions, TLSOptions, TokenRefresher } from './options'
import { TokenStore, parseTokenExpiry } from './token-store'
import { sleepUntilOrAbort } from './internal/sleep'
import { Producer, ProducerStub } from './producer'
import { Consumer, ConsumerStub } from './consumer'

// Proto is copied into dist/ by the build script. When running from source via
// ts-node, fall back to the proto file in the repo root.
const PROTO_PATH = fs.existsSync(path.join(__dirname, 'queue.proto'))
  ? path.join(__dirname, 'queue.proto')
  : path.join(__dirname, '../../../proto/queue.proto')

const packageDef = protoLoader.loadSync(PROTO_PATH, {
  keepCase: false,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
  includeDirs: [__dirname],
})

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const proto = grpc.loadPackageDefinition(packageDef) as any

const ADVANCE_WINDOW_MS = 60_000
const RETRY_BACKOFF_START_MS = 5_000
const RETRY_BACKOFF_MAX_MS = 60_000

export class Client {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private readonly stub: any
  private readonly store: TokenStore | null
  private readonly refreshController: AbortController | null = null

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  constructor(stub: any, store: TokenStore | null, refresher?: TokenRefresher) {
    this.stub = stub
    this.store = store

    if (refresher && store) {
      this.refreshController = new AbortController()
      void this.runRefresher(refresher, this.refreshController.signal)
    }
  }

  producer(): Producer {
    return new Producer(this.stub as ProducerStub)
  }

  consumer(topic: string, options?: ConsumerOptions): Consumer {
    return new Consumer(this.stub as ConsumerStub, topic, options)
  }

  close(): void {
    this.refreshController?.abort()
    this.stub.getChannel().close()
  }

  setToken(token: string): void {
    if (!this.store) {
      throw new Error('setToken: client was not created with a bearer token')
    }
    this.store.set(token)
  }

  private async runRefresher(refresher: TokenRefresher, signal: AbortSignal): Promise<void> {
    let retryBackoff = RETRY_BACKOFF_START_MS

    while (!signal.aborted) {
      let sleepMs: number

      try {
        const exp = parseTokenExpiry(this.store!.get())
        const remaining = exp.getTime() - Date.now() - ADVANCE_WINDOW_MS
        if (remaining <= 0) {
          sleepMs = 0
        } else {
          sleepMs = remaining
          retryBackoff = RETRY_BACKOFF_START_MS
        }
      } catch {
        // Can't parse expiry — retry after backoff
        console.error('queue-ti: token refresher: cannot parse token expiry')
        sleepMs = retryBackoff
      }

      if (sleepMs > 0) {
        const slept = await sleepUntilOrAbort(sleepMs, signal)
        if (!slept) return
      }

      if (signal.aborted) return

      let newToken: string
      try {
        newToken = await refresher()
      } catch (err) {
        if (signal.aborted) return
        console.error(`queue-ti: token refresher: refresh failed (retrying in ${retryBackoff}ms):`, err)
        const slept = await sleepUntilOrAbort(retryBackoff, signal)
        if (!slept) return
        retryBackoff = Math.min(retryBackoff * 2, RETRY_BACKOFF_MAX_MS)
        continue
      }

      this.store!.set(newToken)
      retryBackoff = RETRY_BACKOFF_START_MS
    }
  }
}


export async function connect(address: string, options?: ConnectOptions): Promise<Client> {
  let credentials: grpc.ChannelCredentials
  const channelOptions: grpc.ChannelOptions = {}

  if (options?.insecure) {
    credentials = grpc.credentials.createInsecure()
  } else {
    const tls: TLSOptions | undefined = options?.tls
    credentials = grpc.credentials.createSsl(
      tls?.rootCerts ?? null,
      tls?.privateKey ?? null,
      tls?.certChain ?? null,
    )
    if (tls?.serverNameOverride) {
      channelOptions['grpc.ssl_target_name_override'] = tls.serverNameOverride
    }
  }

  let store: TokenStore | null = null

  if (options?.token) {
    store = new TokenStore(options.token)

    if (options.insecure) {
      // grpc-js forbids composing call credentials with insecure channel credentials.
      // Use a channel interceptor to inject the Authorization header instead.
      channelOptions.interceptors = [
        (interceptorOptions: grpc.InterceptorOptions, nextCall: grpc.NextCall) => {
          return new grpc.InterceptingCall(nextCall(interceptorOptions), {
            start(metadata, listener, next) {
              metadata.add('authorization', `Bearer ${store!.get()}`)
              next(metadata, listener)
            },
          })
        },
      ]
    } else {
      const callCredentials = grpc.credentials.createFromMetadataGenerator((_params, callback) => {
        const meta = new grpc.Metadata()
        meta.add('authorization', `Bearer ${store!.get()}`)
        callback(null, meta)
      })
      credentials = grpc.credentials.combineChannelCredentials(credentials, callCredentials)
    }
  }

  const stub = new proto.queue.QueueService(address, credentials, channelOptions) as unknown

  return new Client(stub, store, options?.tokenRefresher)
}
