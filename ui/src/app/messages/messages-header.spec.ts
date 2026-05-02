import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { provideRouter } from '@angular/router';
import { MessagesHeader } from './messages-header';
import { AuthService } from '../services/auth.service';

const makeAuthService = (isAdmin: boolean) =>
  ({
    isAdmin: () => isAdmin,
    isAuthenticated: () => true,
  }) as unknown as AuthService;

const setup = async (isAdmin: boolean) => {
  await TestBed.configureTestingModule({
    imports: [MessagesHeader],
    providers: [
      provideZonelessChangeDetection(),
      provideRouter([]),
      { provide: AuthService, useValue: makeAuthService(isAdmin) },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(MessagesHeader);
  fixture.componentRef.setInput('isAuthenticated', true);
  fixture.detectChanges();
  await fixture.whenStable();
  return { fixture };
};

describe('MessagesHeader', () => {
  describe('when the user is an admin', () => {
    it('should show the Admin navigation link', async () => {
      const { fixture } = await setup(true);
      const el: HTMLElement = fixture.nativeElement;
      const links = Array.from(el.querySelectorAll('a'));
      expect(links.some((a) => a.textContent?.trim() === 'Admin')).toBe(true);
    });
  });

  describe('when the user is not an admin', () => {
    it('should not show the Admin navigation link', async () => {
      const { fixture } = await setup(false);
      const el: HTMLElement = fixture.nativeElement;
      const links = Array.from(el.querySelectorAll('a'));
      expect(links.some((a) => a.textContent?.trim() === 'Admin')).toBe(false);
    });
  });

  describe('when authenticated', () => {
    it('should show the Messages navigation link', async () => {
      const { fixture } = await setup(false);
      const el: HTMLElement = fixture.nativeElement;
      const links = Array.from(el.querySelectorAll('a'));
      expect(links.some((a) => a.textContent?.trim() === 'Messages')).toBe(true);
    });

    it('should show the Sign out button', async () => {
      const { fixture } = await setup(false);
      const el: HTMLElement = fixture.nativeElement;
      const buttons = Array.from(el.querySelectorAll('button'));
      expect(buttons.some((b) => b.textContent?.trim().includes('Sign out'))).toBe(true);
    });
  });
});
