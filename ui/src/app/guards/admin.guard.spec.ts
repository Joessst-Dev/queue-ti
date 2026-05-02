import { TestBed } from '@angular/core/testing';
import { Router, UrlTree } from '@angular/router';
import { adminGuard } from './admin.guard';
import { AuthService } from '../services/auth.service';

const makeAuthService = (isAdmin: boolean) =>
  ({
    isAdmin: () => isAdmin,
  }) as unknown as AuthService;

const fakeRouter = () => ({
  createUrlTree: (commands: string[]) => ({ commands }) as unknown as UrlTree,
  navigate: vi.fn(),
});

describe('adminGuard', () => {
  describe('when the user is an admin', () => {
    it('should return true (allow access)', () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService(true) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        result = adminGuard({} as never, {} as never) as boolean | UrlTree;
      });

      expect(result).toBe(true);
    });
  });

  describe('when the user is not an admin', () => {
    it('should redirect to /messages', () => {
      TestBed.configureTestingModule({
        providers: [
          { provide: AuthService, useValue: makeAuthService(false) },
          { provide: Router, useValue: fakeRouter() },
        ],
      });

      let result: boolean | UrlTree | undefined;
      TestBed.runInInjectionContext(() => {
        result = adminGuard({} as never, {} as never) as boolean | UrlTree;
      });

      const urlTree = result as unknown as { commands: string[] };
      expect(urlTree.commands).toEqual(['/messages']);
    });
  });
});
