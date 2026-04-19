import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';

export interface QueueMessage {
  id: string;
  topic: string;
  payload: string;
  metadata: Record<string, string>;
  status: string;
  created_at: string;
}

export interface EnqueueRequest {
  topic: string;
  payload: string;
  metadata: Record<string, string>;
}

@Injectable({ providedIn: 'root' })
export class QueueService {
  private http = inject(HttpClient);

  listMessages(topic?: string) {
    const params: Record<string, string> = {};
    if (topic) {
      params['topic'] = topic;
    }
    return this.http.get<QueueMessage[]>('/api/messages', { params });
  }

  enqueueMessage(req: EnqueueRequest) {
    return this.http.post<{ id: string }>('/api/messages', req);
  }
}
