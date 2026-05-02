import { TestBed } from '@angular/core/testing';
import {
  HttpClient,
  provideHttpClient,
  withInterceptors,
} from '@angular/common/http';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { Router } from '@angular/router';
import { authInterceptor } from './auth.interceptor';
import { AuthService } from '../services/auth.service';

const makeAuthService = (overrides: {
  authHeader: string | null;
  isAuthenticated: boolean;
}) =>
  ({
    getAuthHeader: () => overrides.authHeader,
    isAuthenticated: () => overrides.isAuthenticated,
    logout: vi.fn(),
  }) as unknown as AuthService;

describe('authInterceptor', () => {
  let http: HttpClient;
  let httpController: HttpTestingController;
  let router: { navigate: ReturnType<typeof vi.fn> };
  let authService: ReturnType<typeof makeAuthService>;

  const setup = (opts: { authHeader: string | null; isAuthenticated: boolean }) => {
    authService = makeAuthService(opts);
    router = { navigate: vi.fn() };

    TestBed.configureTestingModule({
      providers: [
        { provide: AuthService, useValue: authService },
        { provide: Router, useValue: router },
        provideHttpClient(withInterceptors([authInterceptor])),
        provideHttpClientTesting(),
      ],
    });

    http = TestBed.inject(HttpClient);
    httpController = TestBed.inject(HttpTestingController);
  };

  afterEach(() => {
    httpController.verify();
  });

  describe('when getAuthHeader() returns a Bearer token and request has no Authorization header', () => {
    it('should clone the request and add the Authorization header', () => {
      setup({ authHeader: 'Bearer test.jwt.token', isAuthenticated: true });

      http.get('/api/messages').subscribe();

      const req = httpController.expectOne('/api/messages');
      expect(req.request.headers.get('Authorization')).toBe('Bearer test.jwt.token');
      req.flush([]);
    });
  });

  describe('when the request already has an Authorization header', () => {
    it('should not overwrite the existing header', () => {
      setup({ authHeader: 'Bearer test.jwt.token', isAuthenticated: true });

      http
        .get('/api/messages', {
          headers: { Authorization: 'Bearer other.existing.token' },
        })
        .subscribe();

      const req = httpController.expectOne('/api/messages');
      expect(req.request.headers.get('Authorization')).toBe(
        'Bearer other.existing.token',
      );
      req.flush([]);
    });
  });

  describe('when getAuthHeader() returns null', () => {
    it('should pass the request through without adding an Authorization header', () => {
      setup({ authHeader: null, isAuthenticated: false });

      http.get('/api/messages').subscribe();

      const req = httpController.expectOne('/api/messages');
      expect(req.request.headers.has('Authorization')).toBe(false);
      req.flush([]);
    });
  });

  describe('on HTTP 401 response', () => {
    describe('when user is authenticated', () => {
      it('should call auth.logout()', () => {
        setup({ authHeader: 'Bearer test.jwt.token', isAuthenticated: true });

        http.get('/api/messages').subscribe({ error: vi.fn() });
        httpController
          .expectOne('/api/messages')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(authService.logout).toHaveBeenCalledOnce();
      });

      it('should navigate to /login', () => {
        setup({ authHeader: 'Bearer test.jwt.token', isAuthenticated: true });

        http.get('/api/messages').subscribe({ error: vi.fn() });
        httpController
          .expectOne('/api/messages')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(router.navigate).toHaveBeenCalledWith(['/login']);
      });
    });

    describe('when user is not authenticated', () => {
      it('should not call logout or navigate', () => {
        setup({ authHeader: null, isAuthenticated: false });

        http.get('/api/messages').subscribe({ error: vi.fn() });
        httpController
          .expectOne('/api/messages')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(authService.logout).not.toHaveBeenCalled();
        expect(router.navigate).not.toHaveBeenCalled();
      });
    });
  });
});
