import { TestBed } from '@angular/core/testing';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideZonelessChangeDetection } from '@angular/core';
import { AuthService } from './auth.service';

const makeJwt = (payload: object): string =>
  `header.${btoa(JSON.stringify(payload))}.sig`;

describe('AuthService', () => {
  let service: AuthService;
  let httpController: HttpTestingController;

  beforeEach(() => {
    sessionStorage.clear();
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideZonelessChangeDetection(),
      ],
    });
    service = TestBed.inject(AuthService);
    httpController = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpController.verify();
    sessionStorage.clear();
  });

  describe('login()', () => {
    describe('when the server responds with a token', () => {
      const jwt = makeJwt({ sub: 'admin', adm: true });

      it('should POST to /api/auth/login with username and password', () => {
        service.login('admin', 'secret').subscribe();

        const req = httpController.expectOne('/api/auth/login');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ username: 'admin', password: 'secret' });
        req.flush({ token: jwt });
      });

      it('should store the token in sessionStorage', () => {
        service.login('admin', 'secret').subscribe();
        httpController.expectOne('/api/auth/login').flush({ token: jwt });

        expect(sessionStorage.getItem('queueti_jwt')).toBe(jwt);
      });

      it('should set isAuthenticated to true', () => {
        service.login('admin', 'secret').subscribe();
        httpController.expectOne('/api/auth/login').flush({ token: jwt });

        expect(service.isAuthenticated()).toBe(true);
      });

      it('should return true', () => {
        let result: boolean | undefined;
        service.login('admin', 'secret').subscribe((v) => (result = v));
        httpController.expectOne('/api/auth/login').flush({ token: jwt });

        expect(result).toBe(true);
      });
    });

    describe('when the server responds with 401', () => {
      it('should return false', () => {
        let result: boolean | undefined;
        service.login('admin', 'wrong').subscribe((v) => (result = v));
        httpController
          .expectOne('/api/auth/login')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(result).toBe(false);
      });

      it('should not touch sessionStorage', () => {
        service.login('admin', 'wrong').subscribe();
        httpController
          .expectOne('/api/auth/login')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(sessionStorage.getItem('queueti_jwt')).toBeNull();
      });

      it('should leave isAuthenticated false', () => {
        service.login('admin', 'wrong').subscribe();
        httpController
          .expectOne('/api/auth/login')
          .flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });

        expect(service.isAuthenticated()).toBe(false);
      });
    });
  });

  describe('logout()', () => {
    beforeEach(() => {
      const jwt = makeJwt({ sub: 'admin', adm: false });
      service.login('admin', 'secret').subscribe();
      httpController.expectOne('/api/auth/login').flush({ token: jwt });
    });

    it('should remove queueti_jwt from sessionStorage', () => {
      service.logout();
      expect(sessionStorage.getItem('queueti_jwt')).toBeNull();
    });

    it('should set isAuthenticated to false', () => {
      service.logout();
      expect(service.isAuthenticated()).toBe(false);
    });
  });

  describe('isAdmin()', () => {
    describe('when token has adm: true', () => {
      it('should return true', () => {
        const jwt = makeJwt({ sub: 'admin', adm: true });
        service.login('admin', 'secret').subscribe();
        httpController.expectOne('/api/auth/login').flush({ token: jwt });

        expect(service.isAdmin()).toBe(true);
      });
    });

    describe('when token has adm: false', () => {
      it('should return false', () => {
        const jwt = makeJwt({ sub: 'user', adm: false });
        service.login('user', 'pass').subscribe();
        httpController.expectOne('/api/auth/login').flush({ token: jwt });

        expect(service.isAdmin()).toBe(false);
      });
    });

    describe('when not authenticated', () => {
      it('should return false', () => {
        expect(service.isAdmin()).toBe(false);
      });
    });
  });

  describe('currentUsername()', () => {
    describe('when authenticated', () => {
      it('should return the sub field from the token payload', () => {
        const jwt = makeJwt({ sub: 'alice', adm: false });
        service.login('alice', 'pass').subscribe();
        httpController.expectOne('/api/auth/login').flush({ token: jwt });

        expect(service.currentUsername()).toBe('alice');
      });
    });

    describe('when not authenticated', () => {
      it('should return null', () => {
        expect(service.currentUsername()).toBeNull();
      });
    });
  });

  describe('getAuthHeader()', () => {
    describe('when not authenticated', () => {
      it('should return null', () => {
        expect(service.getAuthHeader()).toBeNull();
      });
    });

    describe('when authenticated', () => {
      it('should return "Bearer <token>"', () => {
        const jwt = makeJwt({ sub: 'admin', adm: true });
        service.login('admin', 'secret').subscribe();
        httpController.expectOne('/api/auth/login').flush({ token: jwt });

        expect(service.getAuthHeader()).toBe(`Bearer ${jwt}`);
      });
    });
  });

  describe('checkAuthStatus()', () => {
    describe('when server responds with auth_required: true', () => {
      it('should return true', () => {
        let result: boolean | undefined;
        service.checkAuthStatus().subscribe((v) => (result = v));
        httpController.expectOne('/api/auth/status').flush({ auth_required: true });

        expect(result).toBe(true);
      });

      it('should set authRequired signal to true', () => {
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

    describe('when the request errors', () => {
      it('should return false and set authRequired to false', () => {
        let result: boolean | undefined;
        service.checkAuthStatus().subscribe((v) => (result = v));
        httpController
          .expectOne('/api/auth/status')
          .flush('Server Error', { status: 500, statusText: 'Internal Server Error' });

        expect(result).toBe(false);
        expect(service.authRequired()).toBe(false);
      });
    });
  });

  describe('refreshToken()', () => {
    describe('when the server responds with a new token', () => {
      it('should POST to /api/auth/refresh', () => {
        service.refreshToken().subscribe();

        const req = httpController.expectOne('/api/auth/refresh');
        expect(req.request.method).toBe('POST');
        req.flush({ token: makeJwt({ sub: 'admin', adm: true }) });
      });

      it('should update the stored token in sessionStorage', () => {
        const newJwt = makeJwt({ sub: 'admin', adm: true });
        service.refreshToken().subscribe();
        httpController.expectOne('/api/auth/refresh').flush({ token: newJwt });

        expect(sessionStorage.getItem('queueti_jwt')).toBe(newJwt);
      });

      it('should update the token signal so isAuthenticated remains true', () => {
        const newJwt = makeJwt({ sub: 'admin', adm: true });
        service.refreshToken().subscribe();
        httpController.expectOne('/api/auth/refresh').flush({ token: newJwt });

        expect(service.isAuthenticated()).toBe(true);
      });
    });
  });
});
