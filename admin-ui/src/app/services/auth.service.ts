import { Injectable, inject, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, of, map, tap, catchError } from 'rxjs';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);

  private credentials = signal<string | null>(
    sessionStorage.getItem('queueti_auth'),
  );
  private _authRequired = signal<boolean | null>(null);

  isAuthenticated = computed(() => this.credentials() !== null);
  authRequired = computed(() => this._authRequired());

  checkAuthStatus(): Observable<boolean> {
    if (this._authRequired() !== null) {
      return of(this._authRequired() as boolean);
    }
    return this.http.get<{ auth_required: boolean }>('/api/auth/status').pipe(
      map((resp) => resp.auth_required),
      tap((required) => this._authRequired.set(required)),
    );
  }

  login(username: string, password: string): Observable<boolean> {
    const token = btoa(`${username}:${password}`);
    return this.http
      .get('/api/messages', {
        headers: { Authorization: `Basic ${token}` },
      })
      .pipe(
        tap(() => {
          sessionStorage.setItem('queueti_auth', token);
          this.credentials.set(token);
        }),
        map(() => true),
        catchError(() => of(false)),
      );
  }

  logout(): void {
    sessionStorage.removeItem('queueti_auth');
    this.credentials.set(null);
  }

  getAuthHeader(): string | null {
    const creds = this.credentials();
    return creds ? `Basic ${creds}` : null;
  }
}
