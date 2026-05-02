import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { provideRouter } from '@angular/router';
import { Router } from '@angular/router';
import { Observable, of } from 'rxjs';
import { Login } from './login';
import { AuthService } from '../services/auth.service';

const makeAuthService = (loginResult: boolean) =>
  ({
    login: vi.fn().mockReturnValue(of(loginResult)),
    isAuthenticated: () => false,
    getAuthHeader: () => null,
  }) as unknown as AuthService;

const setup = async (loginResult = true) => {
  await TestBed.configureTestingModule({
    imports: [Login],
    providers: [
      provideZonelessChangeDetection(),
      provideRouter([]),
      { provide: AuthService, useValue: makeAuthService(loginResult) },
    ],
  }).compileComponents();

  const router = TestBed.inject(Router);
  vi.spyOn(router, 'navigate').mockResolvedValue(true);

  const fixture = TestBed.createComponent(Login);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  return { fixture, router };
};

describe('Login', () => {
  describe('initial render', () => {
    it('should render a username input', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;
      expect(el.querySelector('input#username')).not.toBeNull();
    });

    it('should render a password input', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;
      const input = el.querySelector<HTMLInputElement>('input#password');
      expect(input).not.toBeNull();
      expect(input?.type).toBe('password');
    });

    it('should render a submit button', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;
      const button = el.querySelector<HTMLButtonElement>('button[type="submit"]');
      expect(button).not.toBeNull();
    });

    it('should not show an error message initially', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;
      const errorDivs = el.querySelectorAll('.bg-red-50');
      expect(errorDivs.length).toBe(0);
    });
  });

  describe('when login succeeds', () => {
    it('should navigate to /messages', async () => {
      const { fixture, router } = await setup(true);
      const el: HTMLElement = fixture.nativeElement;

      el.querySelector('form')?.dispatchEvent(new Event('submit'));
      await fixture.whenStable();
      fixture.detectChanges();

      expect(router.navigate).toHaveBeenCalledWith(['/messages']);
    });
  });

  describe('when login fails', () => {
    it('should show "Invalid credentials" error message', async () => {
      const { fixture } = await setup(false);
      const el: HTMLElement = fixture.nativeElement;

      el.querySelector('form')?.dispatchEvent(new Event('submit'));
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).toContain('Invalid credentials');
    });

    it('should not navigate away', async () => {
      const { fixture, router } = await setup(false);
      const el: HTMLElement = fixture.nativeElement;

      el.querySelector('form')?.dispatchEvent(new Event('submit'));
      await fixture.whenStable();
      fixture.detectChanges();

      expect(router.navigate).not.toHaveBeenCalled();
    });
  });

  describe('while loading (between submit and response)', () => {
    it('should disable the submit button', async () => {
      const neverCompleting$ = new Observable<boolean>(() => {
        // Never emits — simulates an in-flight request
      });

      const authService = {
        login: vi.fn().mockReturnValue(neverCompleting$),
        isAuthenticated: () => false,
        getAuthHeader: () => null,
      } as unknown as AuthService;

      await TestBed.configureTestingModule({
        imports: [Login],
        providers: [
          provideZonelessChangeDetection(),
          provideRouter([]),
          { provide: AuthService, useValue: authService },
        ],
      }).compileComponents();

      const fixture = TestBed.createComponent(Login);
      fixture.detectChanges();
      await fixture.whenStable();

      const el: HTMLElement = fixture.nativeElement;
      el.querySelector('form')?.dispatchEvent(new Event('submit'));
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.detectChanges();

      const button = el.querySelector<HTMLButtonElement>('button[type="submit"]');
      expect(button?.disabled).toBe(true);
    });
  });
});
