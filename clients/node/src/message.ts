export interface Message {
  id: string
  topic: string
  payload: Buffer
  metadata: Record<string, string>
  createdAt: Date
  retryCount: number
  ack(): Promise<void>
  nack(reason: string): Promise<void>
}

export function buildMessage(
  id: string,
  topic: string,
  payload: Buffer,
  metadata: Record<string, string>,
  createdAt: Date,
  retryCount: number,
  ackFn: (id: string) => Promise<void>,
  nackFn: (id: string, reason: string) => Promise<void>,
): Message {
  return {
    id,
    topic,
    payload,
    metadata,
    createdAt,
    retryCount,
    ack: () => ackFn(id),
    nack: (reason: string) => nackFn(id, reason),
  }
}
