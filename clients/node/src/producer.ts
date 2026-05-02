import { PublishOptions } from './options'

// GrpcStub is the minimal shape of the gRPC stub the Producer needs.
export interface ProducerStub {
  enqueue(
    request: { topic: string; payload: Buffer; metadata: Record<string, string>; key?: string },
    callback: (err: Error | null, response: { id: string }) => void,
  ): void
}

export class Producer {
  constructor(private readonly stub: ProducerStub) {}

  publish(topic: string, payload: Buffer | Uint8Array, options?: PublishOptions): Promise<string> {
    const buf = Buffer.isBuffer(payload) ? payload : Buffer.from(payload)
    const req: { topic: string; payload: Buffer; metadata: Record<string, string>; key?: string } = {
      topic,
      payload: buf,
      metadata: options?.metadata ?? {},
    }
    if (options?.key !== undefined) {
      req.key = options.key
    }

    return new Promise<string>((resolve, reject) => {
      this.stub.enqueue(req, (err, response) => {
        if (err) {
          reject(new Error(`publish to topic "${topic}": ${err.message}`))
        } else {
          resolve(response.id)
        }
      })
    })
  }
}
