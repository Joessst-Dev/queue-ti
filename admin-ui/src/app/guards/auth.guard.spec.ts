import { TestBed } from '@angular/core/testing';
import { Router, UrlTree } from '@angular/router';
import { of } from 'rxjs';
import { authGuard, loginGuard } from './auth.guard';
import { AuthService } from '../services/auth.service';

const makeAuthService = (opts: {
  checkAuthStatusResult: boolean;
  isAuthenticated: boolean;
}) =>
  ({
    checkAuthStatus: () => of(opts.checkAuthStatusResult),
    isAuthenticated: () => opts.isAuthenticated,
  }) as unknown as AuthService;

const fakeRouter = () => ({
  createUrlTree: (commands: string[]) => ({ commands }) as unknown as UrlTree,
  navigate: vi.fn(),
});

describe('authGuard', () => {
  describe('when checkAuthStatus returns false (auth not required)', () => {
    it('should return true (allow access)', async () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService({ checkAuthStatusResult: false, isAuthenticated: false }) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        (authGuard({} as never, {} as never) as ReturnType<typeof of>).subscribe(
          (v: boolean | UrlTree) => (result = v),
        );
      });

      expect(result).toBe(true);
    });
  });

  describe('when auth is required and user is authenticated', () => {
    it('should return true (allow access)', () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService({ checkAuthStatusResult: true, isAuthenticated: true }) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        (authGuard({} as never, {} as never) as ReturnType<typeof of>).subscribe(
          (v: boolean | UrlTree) => (result = v),
        );
      });

      expect(result).toBe(true);
    });
  });

  describe('when auth is required and user is not authenticated', () => {
    it('should return a UrlTree for /login', () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService({ checkAuthStatusResult: true, isAuthenticated: false }) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        (authGuard({} as never, {} as never) as ReturnType<typeof of>).subscribe(
          (v: boolean | UrlTree) => (result = v),
        );
      });

      const urlTree = result as unknown as { commands: string[] };
      expect(urlTree.commands).toEqual(['/login']);
    });
  });
});

describe('loginGuard', () => {
  describe('when auth is not required', () => {
    it('should redirect to /messages', () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService({ checkAuthStatusResult: false, isAuthenticated: false }) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        (loginGuard({} as never, {} as never) as ReturnType<typeof of>).subscribe(
          (v: boolean | UrlTree) => (result = v),
        );
      });

      const urlTree = result as unknown as { commands: string[] };
      expect(urlTree.commands).toEqual(['/messages']);
    });
  });

  describe('when auth is required and user is authenticated', () => {
    it('should redirect to /messages', () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService({ checkAuthStatusResult: true, isAuthenticated: true }) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        (loginGuard({} as never, {} as never) as ReturnType<typeof of>).subscribe(
          (v: boolean | UrlTree) => (result = v),
        );
      });

      const urlTree = result as unknown as { commands: string[] };
      expect(urlTree.commands).toEqual(['/messages']);
    });
  });

  describe('when auth is required and user is not authenticated', () => {
    it('should return true (allow access to login page)', () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService({ checkAuthStatusResult: true, isAuthenticated: false }) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        (loginGuard({} as never, {} as never) as ReturnType<typeof of>).subscribe(
          (v: boolean | UrlTree) => (result = v),
        );
      });

      expect(result).toBe(true);
    });
  });
});
