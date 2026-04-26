import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

export interface User {
  id: string;
  username: string;
  is_admin: boolean;
  created_at: string;
  updated_at: string;
}

export interface Grant {
  id: string;
  user_id: string;
  action: 'read' | 'write' | 'admin';
  topic_pattern: string;
  created_at: string;
}

export interface CreateUserRequest {
  username: string;
  password: string;
  is_admin: boolean;
}

export interface UpdateUserRequest {
  username?: string;
  password?: string;
  is_admin?: boolean;
}

export interface AddGrantRequest {
  action: 'read' | 'write' | 'admin';
  topic_pattern: string;
}

@Injectable({ providedIn: 'root' })
export class UserService {
  private http = inject(HttpClient);

  listUsers(): Observable<User[]> {
    return this.http
      .get<{ items: User[] }>('/api/users')
      .pipe(map((r) => r.items ?? []));
  }

  createUser(req: CreateUserRequest): Observable<User> {
    return this.http.post<User>('/api/users', req);
  }

  updateUser(id: string, req: UpdateUserRequest): Observable<User> {
    return this.http.put<User>(`/api/users/${id}`, req);
  }

  deleteUser(id: string): Observable<void> {
    return this.http.delete<void>(`/api/users/${id}`);
  }

  listGrants(userId: string): Observable<Grant[]> {
    return this.http
      .get<{ items: Grant[] }>(`/api/users/${userId}/grants`)
      .pipe(map((r) => r.items ?? []));
  }

  addGrant(userId: string, req: AddGrantRequest): Observable<Grant> {
    return this.http.post<Grant>(`/api/users/${userId}/grants`, req);
  }

  deleteGrant(userId: string, grantId: string): Observable<void> {
    return this.http.delete<void>(`/api/users/${userId}/grants/${grantId}`);
  }
}
