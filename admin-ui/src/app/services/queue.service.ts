import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

export interface QueueMessage {
  id: string;
  topic: string;
  payload: string;
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
}
