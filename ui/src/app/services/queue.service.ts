import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

export interface TopicSchema {
  topic: string;
  schema_json: string;
  version: number;
  updated_at: string;
}

export interface TopicSchemasResponse {
  items: TopicSchema[];
}

export interface QueueMessage {
  id: string;
  topic: string;
  payload: string;
  key?: string;
  metadata: Record<string, string>;
  status: string;
  created_at: string;
  retry_count: number;
  max_retries: number;
  last_error: string;
  expires_at: string | null;
  original_topic: string | null;
  dlq_moved_at: string | null;
}

export interface EnqueueRequest {
  topic: string;
  payload: string;
  key?: string;
  metadata: Record<string, string>;
}

export interface TopicStat {
  topic: string;
  status: string;
  count: number;
}

export interface StatsResponse {
  topics: TopicStat[];
}

export interface PagedMessages {
  items: QueueMessage[];
  total: number;
  limit: number;
  offset: number;
}

export interface TopicConfig {
  topic: string;
  max_retries?: number | null;
  message_ttl_seconds?: number | null;
  max_depth?: number | null;
  replayable?: boolean;
  replay_window_seconds?: number | null;
  throughput_limit?: number | null;
}

export interface ReplayResponse {
  topic: string;
  enqueued: number;
  from_time: string;
}

export interface ArchivedMessage {
  id: string;
  topic: string;
  key?: string;
  payload: string;
  retry_count: number;
  original_topic?: string;
  created_at: string;
  acked_at: string;
}

export interface MessageLogResponse {
  items: ArchivedMessage[];
  total: number;
  limit: number;
  offset: number;
}

export interface TopicConfigsResponse {
  items: TopicConfig[];
}

export interface DeleteReaperSchedule {
  schedule: string;
  active: boolean;
}

export interface ConsumerGroupsResponse {
  items: string[];
}

export const PAGE_SIZE = 50;

@Injectable({ providedIn: 'root' })
export class QueueService {
  private http = inject(HttpClient);

  listMessages(topic?: string, offset = 0): Observable<PagedMessages> {
    const params: Record<string, string | number> = { limit: PAGE_SIZE, offset };
    if (topic) {
      params['topic'] = topic;
    }
    return this.http.get<PagedMessages>('/api/messages', { params });
  }

  enqueueMessage(req: EnqueueRequest) {
    return this.http.post<{ id: string }>('/api/messages', req);
  }

  nackMessage(id: string, error?: string) {
    return this.http.post<void>(`/api/messages/${id}/nack`, { error: error ?? '' });
  }

  requeueMessage(id: string) {
    return this.http.post<void>(`/api/messages/${id}/requeue`, {});
  }

  getStats(): Observable<StatsResponse> {
    return this.http.get<StatsResponse>('/api/stats');
  }

  getTopicConfigs(): Observable<TopicConfigsResponse> {
    return this.http.get<TopicConfigsResponse>('/api/topic-configs');
  }

  upsertTopicConfig(topic: string, cfg: Omit<TopicConfig, 'topic'>): Observable<TopicConfig> {
    return this.http.put<TopicConfig>(`/api/topic-configs/${topic}`, cfg);
  }

  deleteTopicConfig(topic: string): Observable<void> {
    return this.http.delete<void>(`/api/topic-configs/${topic}`);
  }

  getTopicSchemas(): Observable<TopicSchema[]> {
    return this.http.get<TopicSchemasResponse>('/api/topic-schemas').pipe(
      map((r) => r.items ?? []),
    );
  }

  getTopicSchema(topic: string): Observable<TopicSchema> {
    return this.http.get<TopicSchema>(`/api/topic-schemas/${topic}`);
  }

  upsertTopicSchema(topic: string, schemaJson: string): Observable<TopicSchema> {
    return this.http.put<TopicSchema>(`/api/topic-schemas/${topic}`, { schema_json: schemaJson });
  }

  deleteTopicSchema(topic: string): Observable<void> {
    return this.http.delete<void>(`/api/topic-schemas/${topic}`);
  }

  purgeTopic(topic: string, statuses: string[]): Observable<{ deleted: number }> {
    return this.http.post<{ deleted: number }>(`/api/topics/${topic}/purge`, { statuses });
  }

  purgeByKey(topic: string, key: string): Observable<{ deleted: number }> {
    return this.http.delete<{ deleted: number }>(`/api/topics/${topic}/messages/by-key/${encodeURIComponent(key)}`);
  }

  runExpiryReaper(): Observable<{ expired: number }> {
    return this.http.post<{ expired: number }>('/api/admin/expiry-reaper/run', {});
  }

  runDeleteReaper(): Observable<{ deleted: number }> {
    return this.http.post<{ deleted: number }>('/api/admin/delete-reaper/run', {});
  }

  replayTopic(topic: string, fromTime?: string): Observable<ReplayResponse> {
    const body: { from_time?: string } = fromTime ? { from_time: fromTime } : {};
    return this.http.post<ReplayResponse>(`/api/topics/${topic}/replay`, body);
  }

  listMessageLog(topic: string, offset = 0): Observable<MessageLogResponse> {
    return this.http.get<MessageLogResponse>(`/api/topics/${topic}/message-log`, {
      params: { limit: PAGE_SIZE, offset },
    });
  }

  trimMessageLog(topic: string, before: string): Observable<{ deleted: number }> {
    return this.http.delete<{ deleted: number }>(`/api/topics/${topic}/message-log`, {
      params: { before },
    });
  }

  runArchiveReaper(): Observable<{ deleted: number }> {
    return this.http.post<{ deleted: number }>('/api/admin/archive-reaper/run', {});
  }

  getDeleteReaperSchedule(): Observable<DeleteReaperSchedule> {
    return this.http.get<DeleteReaperSchedule>('/api/admin/delete-reaper/schedule');
  }

  updateDeleteReaperSchedule(schedule: string): Observable<DeleteReaperSchedule> {
    return this.http.put<DeleteReaperSchedule>('/api/admin/delete-reaper/schedule', { schedule });
  }

  listConsumerGroups(topic: string): Observable<ConsumerGroupsResponse> {
    return this.http.get<ConsumerGroupsResponse>(`/api/topics/${topic}/consumer-groups`);
  }

  registerConsumerGroup(topic: string, group: string): Observable<void> {
    return this.http.post<void>(`/api/topics/${topic}/consumer-groups`, { consumer_group: group });
  }

  unregisterConsumerGroup(topic: string, group: string): Observable<void> {
    return this.http.delete<void>(`/api/topics/${topic}/consumer-groups/${encodeURIComponent(group)}`);
  }
}
