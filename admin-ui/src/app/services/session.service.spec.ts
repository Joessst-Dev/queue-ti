import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { signal } from '@angular/core';
import { provideZonelessChangeDetection } from '@angular/core';
import { Observable, of, throwError } from 'rxjs';
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import { SessionService } from './session.service';
import { AuthService } from './auth.service';

const TOKEN_LIFETIME_MS = 15 * 60 * 1000;
const WARNING_THRESHOLD_MS = 2 * 60 * 1000;
const REFRESH_AFTER_MS = TOKEN_LIFETIME_MS / 2;

interface MockAuth {
  isAuthenticated: ReturnType<typeof signal<boolean>>;
  tokenExpiresAt: ReturnType<typeof signal<number | null>>;
  logout: () => void;
  refreshToken: () => Observable<void>;
}

function makeAuth(overrides: Partial<MockAuth> = {}): MockAuth {
  return {
    isAuthenticated: signal(true),
    tokenExpiresAt: signal(Date.now() + TOKEN_LIFETIME_MS),
    logout: vi.fn() as unknown as () => void,
    refreshToken: vi.fn().mockReturnValue(of(undefined)) as unknown as () => Observable<void>,
    ...overrides,
  };
}

function setup(auth: MockAuth = makeAuth()): {
  service: SessionService;
  router: { navigate: ReturnType<typeof vi.fn> };
} {
  const router = { navigate: vi.fn() };

  TestBed.configureTestingModule({
    providers: [
      provideZonelessChangeDetection(),
      { provide: AuthService, useValue: auth },
      { provide: Router, useValue: router },
    ],
  });

  const service = TestBed.inject(SessionService);
  TestBed.flushEffects();

  return { service, router };
}

describe('SessionService', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    TestBed.resetTestingModule();
  });

  describe('tick() — called immediately via startWith(0)', () => {
    describe('when tokenExpiresAt returns null', () => {
      it('should call auth.logout()', () => {
        const auth = makeAuth({ tokenExpiresAt: signal(null) });
        const { router } = setup(auth);

        expect(auth.logout).toHaveBeenCalledTimes(1);
        expect(router.navigate).toHaveBeenCalledWith(['/login']);
      });

      it('should navigate to /login', () => {
        const auth = makeAuth({ tokenExpiresAt: signal(null) });
        const { router } = setup(auth);

        expect(router.navigate).toHaveBeenCalledWith(['/login']);
      });
    });

    describe('when token is already expired', () => {
      it('should call auth.logout() and navigate to /login', () => {
        const auth = makeAuth({ tokenExpiresAt: signal(Date.now() - 1) });
        const { router } = setup(auth);

        expect(auth.logout).toHaveBeenCalledTimes(1);
        expect(router.navigate).toHaveBeenCalledWith(['/login']);
      });
    });

    describe('when user is active and token is within the refresh threshold', () => {
      it('should call auth.refreshToken()', () => {
        const auth = makeAuth({
          tokenExpiresAt: signal(Date.now() + REFRESH_AFTER_MS - 1),
        });
        setup(auth);

        expect(auth.refreshToken).toHaveBeenCalledTimes(1);
      });
    });

    describe('when user is active and token has plenty of time left', () => {
      it('should not call auth.refreshToken()', () => {
        const auth = makeAuth({
          tokenExpiresAt: signal(Date.now() + TOKEN_LIFETIME_MS),
        });
        setup(auth);

        expect(auth.refreshToken).not.toHaveBeenCalled();
      });
    });

    describe('when user has been inactive for >= 15 min and token expires within 2 min', () => {
      it('should set showWarning to true', () => {
        const now = Date.now();
        const auth = makeAuth({
          tokenExpiresAt: signal(now + TOKEN_LIFETIME_MS + WARNING_THRESHOLD_MS - 1),
        });
        const { service } = setup(auth);

        vi.advanceTimersByTime(TOKEN_LIFETIME_MS);

        expect(service.showWarning()).toBe(true);
      });
    });

    describe('when user has been inactive for >= 15 min and token has > 2 min left', () => {
      it('should not set showWarning to true', () => {
        const now = Date.now();
        const auth = makeAuth({
          tokenExpiresAt: signal(now + TOKEN_LIFETIME_MS + WARNING_THRESHOLD_MS + 60_000),
        });
        const { service } = setup(auth);

        vi.advanceTimersByTime(TOKEN_LIFETIME_MS);

        expect(service.showWarning()).toBe(false);
      });
    });

    describe('when showWarning is true and token time recovers above threshold', () => {
      it('should set showWarning back to false', () => {
        const now = Date.now();
        const expiresAt = signal(now + TOKEN_LIFETIME_MS + WARNING_THRESHOLD_MS - 1);
        const auth = makeAuth({ tokenExpiresAt: expiresAt });
        const { service } = setup(auth);

        vi.advanceTimersByTime(TOKEN_LIFETIME_MS);
        expect(service.showWarning()).toBe(true);

        expiresAt.set(Date.now() + WARNING_THRESHOLD_MS + 60_000);
        vi.advanceTimersByTime(10_000);

        expect(service.showWarning()).toBe(false);
      });
    });

    describe('when refreshToken is already in flight', () => {
      it('should not call refreshToken() again on a subsequent tick', () => {
        const pendingRefresh = new Observable<void>(() => {});

        const auth = makeAuth({
          tokenExpiresAt: signal(Date.now() + REFRESH_AFTER_MS - 1),
          refreshToken: vi.fn().mockReturnValue(pendingRefresh) as unknown as () => Observable<void>,
        });
        setup(auth);

        expect(auth.refreshToken).toHaveBeenCalledTimes(1);

        vi.advanceTimersByTime(10_000);

        expect(auth.refreshToken).toHaveBeenCalledTimes(1);
      });
    });
  });

  describe('extendSession()', () => {
    describe('when refreshToken succeeds', () => {
      it('should call auth.refreshToken()', () => {
        const auth = makeAuth();
        const { service } = setup(auth);

        service.extendSession();

        expect(auth.refreshToken).toHaveBeenCalled();
      });

      it('should set showWarning to false', () => {
        const now = Date.now();
        const expiresAt = signal(now + TOKEN_LIFETIME_MS + WARNING_THRESHOLD_MS - 1);
        const auth = makeAuth({ tokenExpiresAt: expiresAt });
        const { service } = setup(auth);

        vi.advanceTimersByTime(TOKEN_LIFETIME_MS);
        expect(service.showWarning()).toBe(true);

        expiresAt.set(Date.now() + TOKEN_LIFETIME_MS);
        service.extendSession();

        expect(service.showWarning()).toBe(false);
      });

      it('should reset lastActivity so a subsequent tick no longer treats the user as inactive', () => {
        const now = Date.now();
        const expiresAt = signal(now + TOKEN_LIFETIME_MS + WARNING_THRESHOLD_MS - 1);
        const auth = makeAuth({ tokenExpiresAt: expiresAt });
        const { service } = setup(auth);

        vi.advanceTimersByTime(TOKEN_LIFETIME_MS);
        expect(service.showWarning()).toBe(true);

        expiresAt.set(Date.now() + TOKEN_LIFETIME_MS);
        service.extendSession();

        vi.advanceTimersByTime(10_000);

        expect(service.showWarning()).toBe(false);
      });
    });

    describe('when refreshToken errors', () => {
      it('should call auth.logout()', () => {
        const auth = makeAuth({
          refreshToken: vi.fn().mockReturnValue(throwError(() => new Error('network error'))) as unknown as () => Observable<void>,
        });
        const { service } = setup(auth);

        (auth.logout as ReturnType<typeof vi.fn>).mockClear();
        service.extendSession();

        expect(auth.logout).toHaveBeenCalledTimes(1);
      });

      it('should navigate to /login', () => {
        const auth = makeAuth({
          refreshToken: vi.fn().mockReturnValue(throwError(() => new Error('network error'))) as unknown as () => Observable<void>,
        });
        const { router, service } = setup(auth);

        router.navigate.mockClear();
        service.extendSession();

        expect(router.navigate).toHaveBeenCalledWith(['/login']);
      });
    });
  });

  describe('recordActivity()', () => {
    it('should reset msInactive so a subsequent tick no longer considers the user inactive', () => {
      const now = Date.now();
      const expiresAt = signal(now + TOKEN_LIFETIME_MS + WARNING_THRESHOLD_MS - 1);
      const auth = makeAuth({ tokenExpiresAt: expiresAt });
      const { service } = setup(auth);

      vi.advanceTimersByTime(TOKEN_LIFETIME_MS);
      expect(service.showWarning()).toBe(true);

      service.recordActivity();
      expiresAt.set(Date.now() + TOKEN_LIFETIME_MS);
      vi.advanceTimersByTime(10_000);

      expect(service.showWarning()).toBe(false);
    });
  });
});
