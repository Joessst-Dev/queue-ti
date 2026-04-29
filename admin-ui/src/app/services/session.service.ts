import { Injectable, DestroyRef, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Router } from '@angular/router';
import { interval } from 'rxjs';
import { startWith } from 'rxjs/operators';
import { AuthService } from './auth.service';

const TOKEN_LIFETIME_MS = 15 * 60 * 1000;
const WARNING_THRESHOLD_MS = 2 * 60 * 1000;
const REFRESH_AFTER_MS = TOKEN_LIFETIME_MS / 2;

@Injectable({ providedIn: 'root' })
export class SessionService {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);

  readonly showWarning = signal(false);
  readonly secondsRemaining = signal(0);

  private lastActivity = Date.now();
  private isRefreshing = false;

  constructor() {
    effect(() => {
      if (!this.auth.isAuthenticated()) return;

      interval(10_000)
        .pipe(startWith(0), takeUntilDestroyed(this.destroyRef))
        .subscribe(() => this.tick());
    });
  }

  recordActivity(): void {
    this.lastActivity = Date.now();
  }

  extendSession(): void {
    this.auth.refreshToken().subscribe({
      next: () => this.showWarning.set(false),
      error: () => {
        this.auth.logout();
        this.router.navigate(['/login']);
      },
    });
  }

  dismissWarning(): void {
    this.showWarning.set(false);
  }

  private tick(): void {
    const expiresAt = this.auth.tokenExpiresAt();

    if (expiresAt === null || expiresAt <= Date.now()) {
      this.auth.logout();
      this.router.navigate(['/login']);
      return;
    }

    const msUntilExpiry = expiresAt - Date.now();
    const msInactive = Date.now() - this.lastActivity;

    this.secondsRemaining.set(Math.ceil(msUntilExpiry / 1000));

    if (msInactive < TOKEN_LIFETIME_MS && msUntilExpiry < REFRESH_AFTER_MS && !this.isRefreshing) {
      this.isRefreshing = true;
      this.auth.refreshToken().subscribe({
        next: () => {
          this.isRefreshing = false;
        },
        error: () => {
          this.isRefreshing = false;
          this.auth.logout();
          this.router.navigate(['/login']);
        },
      });
    } else if (msUntilExpiry <= WARNING_THRESHOLD_MS && msInactive >= TOKEN_LIFETIME_MS) {
      this.showWarning.set(true);
    } else if (msUntilExpiry > WARNING_THRESHOLD_MS) {
      this.showWarning.set(false);
    }
  }
}
