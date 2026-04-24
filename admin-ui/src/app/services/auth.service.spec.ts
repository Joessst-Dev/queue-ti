import { TestBed } from '@angular/core/testing';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { AuthService } from './auth.service';

describe('AuthService', () => {
  let service: AuthService;
  let httpController: HttpTestingController;

  beforeEach(() => {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(AuthService);
    httpController = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpController.verify();
    sessionStorage.clear();
  });

  describe('isAuthenticated', () => {
    describe('when no credentials are stored', () => {
      it('should return false', () => {
        expect(service.isAuthenticated()).toBe(false);
      });
    });

    describe('when credentials are stored in sessionStorage', () => {
      it('should return true on initial load', () => {
        sessionStorage.setItem('queueti_auth', 'dXNlcjpwYXNz');
        // Re-create service so it picks up the sessionStorage value
        TestBed.resetTestingModule();
        TestBed.configureTestingModule({
          providers: [provideHttpClient(), provideHttpClientTesting()],
        });
        const freshService = TestBed.inject(AuthService);
        expect(freshService.isAuthenticated()).toBe(true);
        TestBed.inject(HttpTestingController).verify();
      });
    });
  });

  describe('checkAuthStatus()', () => {
    describe('when cache is null (first call)', () => {
      it('should hit GET /api/auth/status', () => {
        let result: boolean | undefined;
        service.checkAuthStatus().subscribe((v) => (result = v));

        const req = httpController.expectOne('/api/auth/status');
        expect(req.request.method).toBe('GET');
        req.flush({ auth_required: true });

        expect(result).toBe(true);
      });

      it('should set _authRequired signal to the returned value', () => {
        service.checkAuthStatus().subscribe();
        httpController.expectOne('/api/auth/status').flush({ auth_required: true });

        expect(service.authRequired()).toBe(true);
      });
    });

    describe('when server responds with auth_required: false', () => {
      it('should return false and set authRequired signal to false', () => {
        let result: boolean | undefined;
        service.checkAuthStatus().subscribe((v) => (result = v));
        httpController.expectOne('/api/auth/status').flush({ auth_required: false });

        expect(result).toBe(false);
        expect(service.authRequired()).toBe(false);
      });
    });

    describe('when cache is already populated (second call)', () => {
      it('should return the cached value without making a new HTTP request', () => {
        // First call — populate cache
        service.checkAuthStatus().subscribe();
        httpController.expectOne('/api/auth/status').flush({ auth_required: true });

        // Second call — should use cache
        let result: boolean | undefined;
        service.checkAuthStatus().subscribe((v) => (result = v));
        httpController.expectNone('/api/auth/status');
        expect(result).toBe(true);
      });
    });
  });

  describe('login()', () => {
    const username = 'admin';
    const password = 'secret';
    const expectedToken = btoa(`${username}:${password}`);

    describe('when the server responds with 2xx', () => {
      it('should make GET /api/messages with an Authorization: Basic header', () => {
        service.login(username, password).subscribe();

        const req = httpController.expectOne('/api/messages');
        expect(req.request.method).toBe('GET');
        expect(req.request.headers.get('Authorization')).toBe(
          `Basic ${expectedToken}`,
        );
        req.flush([]);
      });

      it('should persist the token to sessionStorage', () => {
        service.login(username, password).subscribe();
        httpController.expectOne('/api/messages').flush([]);

        expect(sessionStorage.getItem('queueti_auth')).toBe(expectedToken);
      });

      it('should set the credentials signal', () => {
        service.login(username, password).subscribe();
        httpController.expectOne('/api/messages').flush([]);

        expect(service.isAuthenticated()).toBe(true);
        expect(service.getAuthHeader()).toBe(`Basic ${expectedToken}`);
      });

      it('should return true', () => {
        let result: boolean | undefined;
        service.login(username, password).subscribe((v) => (result = v));
        httpController.expectOne('/api/messages').flush([]);

        expect(result).toBe(true);
      });
    });

    describe('when the server responds with an error', () => {
      it('should return false', () => {
        let result: boolean | undefined;
        service.login(username, password).subscribe((v) => (result = v));
        httpController
          .expectOne('/api/messages')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(result).toBe(false);
      });

      it('should not touch sessionStorage', () => {
        service.login(username, password).subscribe();
        httpController
          .expectOne('/api/messages')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(sessionStorage.getItem('queueti_auth')).toBeNull();
      });

      it('should not set credentials', () => {
        service.login(username, password).subscribe();
        httpController
          .expectOne('/api/messages')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(service.isAuthenticated()).toBe(false);
      });
    });
  });

  describe('logout()', () => {
    beforeEach(() => {
      // Simulate an authenticated state
      sessionStorage.setItem('queueti_auth', 'dXNlcjpwYXNz');
      service.login('user', 'pass').subscribe();
      httpController.expectOne('/api/messages').flush([]);
    });

    it('should remove queueti_auth from sessionStorage', () => {
      service.logout();
      expect(sessionStorage.getItem('queueti_auth')).toBeNull();
    });

    it('should set credentials signal to null', () => {
      service.logout();
      expect(service.isAuthenticated()).toBe(false);
    });
  });

  describe('getAuthHeader()', () => {
    describe('when not authenticated', () => {
      it('should return null', () => {
        expect(service.getAuthHeader()).toBeNull();
      });
    });

    describe('when authenticated', () => {
      it('should return "Basic <token>"', () => {
        service.login('user', 'pass').subscribe();
        const token = btoa('user:pass');
        httpController.expectOne('/api/messages').flush([]);

        expect(service.getAuthHeader()).toBe(`Basic ${token}`);
      });
    });
  });
});
