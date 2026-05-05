export interface TopicConfig {
  topic: string
  max_retries?: number | null
  message_ttl_seconds?: number | null
  max_depth?: number | null
  replayable: boolean
  replay_window_seconds?: number | null
  throughput_limit?: number | null
}

export interface TopicConfigInput {
  max_retries?: number | null
  message_ttl_seconds?: number | null
  max_depth?: number | null
  replayable?: boolean
  replay_window_seconds?: number | null
  throughput_limit?: number | null
}

export interface TopicSchema {
  topic: string
  schema_json: string
  version: number
  updated_at: string
}

export interface TopicStat {
  topic: string
  status: string
  count: number
}

export class AdminError extends Error {
  constructor(public readonly statusCode: number, message: string) {
    super(`admin: HTTP ${statusCode}: ${message}`)
    this.name = 'AdminError'
  }
}

export interface AdminOptions {
  token?: string
}

export class AdminClient {
  private readonly baseURL: string
  private readonly token: string | undefined

  constructor(baseURL: string, options?: AdminOptions) {
    this.baseURL = baseURL
    this.token = options?.token
  }

  // ---- Topic config ----------------------------------------------------------

  async listTopicConfigs(): Promise<TopicConfig[]> {
    const result = await this.request<{ items: TopicConfig[] }>('GET', '/api/topic-configs')
    return result?.items ?? []
  }

  async upsertTopicConfig(topic: string, config: TopicConfigInput): Promise<TopicConfig> {
    const result = await this.request<TopicConfig>('PUT', `/api/topic-configs/${encodeURIComponent(topic)}`, config)
    return result!
  }

  async deleteTopicConfig(topic: string): Promise<void> {
    await this.request<null>('DELETE', `/api/topic-configs/${encodeURIComponent(topic)}`)
  }

  // ---- Topic schema ----------------------------------------------------------

  async listTopicSchemas(): Promise<TopicSchema[]> {
    const result = await this.request<{ items: TopicSchema[] }>('GET', '/api/topic-schemas')
    return result?.items ?? []
  }

  async getTopicSchema(topic: string): Promise<TopicSchema> {
    const result = await this.request<TopicSchema>('GET', `/api/topic-schemas/${encodeURIComponent(topic)}`)
    return result!
  }

  async upsertTopicSchema(topic: string, schemaJson: string): Promise<TopicSchema> {
    const result = await this.request<TopicSchema>(
      'PUT',
      `/api/topic-schemas/${encodeURIComponent(topic)}`,
      { schema_json: schemaJson },
    )
    return result!
  }

  async deleteTopicSchema(topic: string): Promise<void> {
    await this.request<null>('DELETE', `/api/topic-schemas/${encodeURIComponent(topic)}`)
  }

  // ---- Consumer groups -------------------------------------------------------

  async listConsumerGroups(topic: string): Promise<string[]> {
    const result = await this.request<{ items: string[] }>(
      'GET',
      `/api/topics/${encodeURIComponent(topic)}/consumer-groups`,
    )
    return result?.items ?? []
  }

  async registerConsumerGroup(topic: string, group: string): Promise<void> {
    await this.request<null>(
      'POST',
      `/api/topics/${encodeURIComponent(topic)}/consumer-groups`,
      { consumer_group: group },
    )
  }

  async unregisterConsumerGroup(topic: string, group: string): Promise<void> {
    await this.request<null>(
      'DELETE',
      `/api/topics/${encodeURIComponent(topic)}/consumer-groups/${encodeURIComponent(group)}`,
    )
  }

  // ---- Stats -----------------------------------------------------------------

  async stats(): Promise<TopicStat[]> {
    const result = await this.request<{ topics: TopicStat[] }>('GET', '/api/stats')
    return result?.topics ?? []
  }

  // ---- Internal --------------------------------------------------------------

  private async request<T>(method: string, path: string, body?: unknown): Promise<T | null> {
    const headers: Record<string, string> = {}

    if (body !== undefined) {
      headers['Content-Type'] = 'application/json'
    }
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`
    }

    const response = await fetch(this.baseURL + path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })

    if (response.status === 204) {
      return null
    }

    if (response.ok) {
      return response.json() as Promise<T>
    }

    const text = await response.text().catch(() => '')
    throw new AdminError(response.status, text || response.statusText)
  }
}
