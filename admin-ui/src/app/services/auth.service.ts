import { Injectable, inject, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, of } from 'rxjs';
import { tap, map, catchError } from 'rxjs/operators';

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);

  private token = signal<string | null>(sessionStorage.getItem('queueti_jwt'));
  private _authRequired = signal<boolean | null>(null);

  isAuthenticated = computed(() => this.token() !== null);
  authRequired = computed(() => this._authRequired());

  tokenExpiresAt = computed<number | null>(() => {
    const t = this.token();
    if (!t) return null;
    try {
      const payload = JSON.parse(atob(t.split('.')[1]));
      return typeof payload['exp'] === 'number' ? payload['exp'] * 1000 : null;
    } catch {
      return null;
    }
  });

  isAdmin = computed(() => {
    const t = this.token();
    if (!t) return false;
    try {
      const payload = JSON.parse(atob(t.split('.')[1]));
      return !!payload['adm'];
    } catch {
      return false;
    }
  });

  currentUsername = computed(() => {
    const t = this.token();
    if (!t) return null;
    try {
      return JSON.parse(atob(t.split('.')[1]))['sub'] as string;
    } catch {
      return null;
    }
  });

  checkAuthStatus(): Observable<boolean> {
    return this.http.get<{ auth_required: boolean }>('/api/auth/status').pipe(
      tap((status) => this._authRequired.set(status.auth_required)),
      map((status) => status.auth_required),
      catchError(() => {
        this._authRequired.set(false);
        return of(false);
      }),
    );
  }

  login(username: string, password: string): Observable<boolean> {
    return this.http
      .post<{ token: string }>('/api/auth/login', { username, password })
      .pipe(
        tap(({ token }) => {
          sessionStorage.setItem('queueti_jwt', token);
          this.token.set(token);
        }),
        map(() => true),
        catchError(() => of(false)),
      );
  }

  logout(): void {
    sessionStorage.removeItem('queueti_jwt');
    this.token.set(null);
  }

  getAuthHeader(): string | null {
    const t = this.token();
    return t ? `Bearer ${t}` : null;
  }

  refreshToken(): Observable<void> {
    return this.http
      .post<{ token: string }>('/api/auth/refresh', {})
      .pipe(
        tap(({ token }) => {
          sessionStorage.setItem('queueti_jwt', token);
          this.token.set(token);
        }),
        map(() => void 0),
      );
  }
}
