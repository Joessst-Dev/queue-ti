import { TestBed } from '@angular/core/testing';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideZonelessChangeDetection } from '@angular/core';
import { UserService, User, Grant } from './user.service';

const makeUser = (overrides: Partial<User> = {}): User => ({
  id: 'user-1',
  username: 'alice',
  is_admin: false,
  created_at: '2024-01-15T10:00:00Z',
  updated_at: '2024-01-15T10:00:00Z',
  ...overrides,
});

const makeGrant = (overrides: Partial<Grant> = {}): Grant => ({
  id: 'grant-1',
  user_id: 'user-1',
  action: 'read',
  topic_pattern: '*',
  created_at: '2024-01-15T10:00:00Z',
  ...overrides,
});

describe('UserService', () => {
  let service: UserService;
  let httpController: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideZonelessChangeDetection(),
      ],
    });
    service = TestBed.inject(UserService);
    httpController = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpController.verify();
  });

  it('listUsers() GETs /api/users and returns items array', () => {
    const users = [makeUser({ id: 'u1' }), makeUser({ id: 'u2' })];
    let result: User[] | undefined;

    service.listUsers().subscribe((v) => (result = v));

    const req = httpController.expectOne('/api/users');
    expect(req.request.method).toBe('GET');
    req.flush({ items: users });

    expect(result).toEqual(users);
  });

  it('listUsers() returns empty array when items is missing', () => {
    let result: User[] | undefined;

    service.listUsers().subscribe((v) => (result = v));
    httpController.expectOne('/api/users').flush({});

    expect(result).toEqual([]);
  });

  it('createUser() POSTs /api/users', () => {
    const req = { username: 'bob', password: 'pass', is_admin: false };
    const created = makeUser({ id: 'u-new', username: 'bob' });
    let result: User | undefined;

    service.createUser(req).subscribe((v) => (result = v));

    const httpReq = httpController.expectOne('/api/users');
    expect(httpReq.request.method).toBe('POST');
    expect(httpReq.request.body).toEqual(req);
    httpReq.flush(created);

    expect(result).toEqual(created);
  });

  it('updateUser() PUTs /api/users/:id', () => {
    const updated = makeUser({ id: 'u1', username: 'alice-updated' });
    let result: User | undefined;

    service.updateUser('u1', { username: 'alice-updated' }).subscribe((v) => (result = v));

    const httpReq = httpController.expectOne('/api/users/u1');
    expect(httpReq.request.method).toBe('PUT');
    expect(httpReq.request.body).toEqual({ username: 'alice-updated' });
    httpReq.flush(updated);

    expect(result).toEqual(updated);
  });

  it('deleteUser() DELETEs /api/users/:id', () => {
    let completed = false;

    service.deleteUser('u1').subscribe(() => (completed = true));

    const httpReq = httpController.expectOne('/api/users/u1');
    expect(httpReq.request.method).toBe('DELETE');
    httpReq.flush(null, { status: 204, statusText: 'No Content' });

    expect(completed).toBe(true);
  });

  it('listGrants() GETs /api/users/:id/grants and returns items array', () => {
    const grants = [makeGrant({ id: 'g1' }), makeGrant({ id: 'g2' })];
    let result: Grant[] | undefined;

    service.listGrants('u1').subscribe((v) => (result = v));

    const req = httpController.expectOne('/api/users/u1/grants');
    expect(req.request.method).toBe('GET');
    req.flush({ items: grants });

    expect(result).toEqual(grants);
  });

  it('listGrants() returns empty array when items is missing', () => {
    let result: Grant[] | undefined;

    service.listGrants('u1').subscribe((v) => (result = v));
    httpController.expectOne('/api/users/u1/grants').flush({});

    expect(result).toEqual([]);
  });

  it('addGrant() POSTs /api/users/:id/grants', () => {
    const req = { action: 'write' as const, topic_pattern: 'orders.*' };
    const created = makeGrant({ id: 'g-new', action: 'write', topic_pattern: 'orders.*' });
    let result: Grant | undefined;

    service.addGrant('u1', req).subscribe((v) => (result = v));

    const httpReq = httpController.expectOne('/api/users/u1/grants');
    expect(httpReq.request.method).toBe('POST');
    expect(httpReq.request.body).toEqual(req);
    httpReq.flush(created);

    expect(result).toEqual(created);
  });

  it('deleteGrant() DELETEs /api/users/:id/grants/:grantId', () => {
    let completed = false;

    service.deleteGrant('u1', 'g1').subscribe(() => (completed = true));

    const httpReq = httpController.expectOne('/api/users/u1/grants/g1');
    expect(httpReq.request.method).toBe('DELETE');
    httpReq.flush(null, { status: 204, statusText: 'No Content' });

    expect(completed).toBe(true);
  });
});
