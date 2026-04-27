import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { MaintenanceSection } from './maintenance-section';
import { QueueService } from '../services/queue.service';

const makeQueueService = (opts: {
  expiryResult?: { expired: number } | 'error';
  deleteResult?: { deleted: number } | 'error';
} = {}) => {
  const { expiryResult = { expired: 0 }, deleteResult = { deleted: 0 } } = opts;

  return {
    runExpiryReaper: vi.fn().mockReturnValue(
      expiryResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to run expiry reaper' } }))
        : of(expiryResult as { expired: number }),
    ),
    runDeleteReaper: vi.fn().mockReturnValue(
      deleteResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to run delete reaper' } }))
        : of(deleteResult as { deleted: number }),
    ),
  } as unknown as QueueService;
};

const setup = async (opts: {
  expiryResult?: { expired: number } | 'error';
  deleteResult?: { deleted: number } | 'error';
} = {}) => {
  const queueService = makeQueueService(opts);

  await TestBed.configureTestingModule({
    imports: [MaintenanceSection],
    providers: [
      provideZonelessChangeDetection(),
      { provide: QueueService, useValue: queueService },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(MaintenanceSection);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  const el: HTMLElement = fixture.nativeElement;
  return { fixture, component: fixture.componentInstance, queueService, el };
};

describe('MaintenanceSection', () => {
  describe('Expiry Reaper', () => {
    describe('when "Run Now" is clicked and succeeds', () => {
      it('should call runExpiryReaper()', async () => {
        const { el, queueService } = await setup({ expiryResult: { expired: 3 } });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[0] as HTMLButtonElement).click();

        expect(queueService.runExpiryReaper).toHaveBeenCalledOnce();
      });

      it('should display the number of messages marked as expired', async () => {
        const { fixture, el } = await setup({ expiryResult: { expired: 5 } });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[0] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('5 messages marked as expired.');
      });

      it('should display zero when no messages were expired', async () => {
        const { fixture, el } = await setup({ expiryResult: { expired: 0 } });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[0] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('0 messages marked as expired.');
      });
    });

    describe('when "Run Now" is clicked and fails', () => {
      it('should display the error message', async () => {
        const { fixture, el } = await setup({ expiryResult: 'error' });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[0] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('Failed to run expiry reaper');
      });

      it('should not display a success message on error', async () => {
        const { fixture, el } = await setup({ expiryResult: 'error' });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[0] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).not.toContain('messages marked as expired');
      });
    });
  });

  describe('Delete Reaper', () => {
    describe('when "Run Now" is clicked and succeeds', () => {
      it('should call runDeleteReaper()', async () => {
        const { el, queueService } = await setup({ deleteResult: { deleted: 2 } });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[1] as HTMLButtonElement).click();

        expect(queueService.runDeleteReaper).toHaveBeenCalledOnce();
      });

      it('should display the number of messages deleted', async () => {
        const { fixture, el } = await setup({ deleteResult: { deleted: 7 } });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[1] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('7 expired messages deleted.');
      });

      it('should display zero when no messages were deleted', async () => {
        const { fixture, el } = await setup({ deleteResult: { deleted: 0 } });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[1] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('0 expired messages deleted.');
      });
    });

    describe('when "Run Now" is clicked and fails', () => {
      it('should display the error message', async () => {
        const { fixture, el } = await setup({ deleteResult: 'error' });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[1] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('Failed to run delete reaper');
      });

      it('should not display a success message on error', async () => {
        const { fixture, el } = await setup({ deleteResult: 'error' });

        const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
          b.textContent?.trim().includes('Run Now'),
        );
        (buttons[1] as HTMLButtonElement).click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).not.toContain('expired messages deleted');
      });
    });
  });

  describe('initial state', () => {
    it('should not show any result or error banners on load', async () => {
      const { el } = await setup();
      expect(el.textContent).not.toContain('messages marked as expired');
      expect(el.textContent).not.toContain('expired messages deleted');
      expect(el.querySelector('.bg-red-50')).toBeNull();
    });
  });
});
