import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { MaintenanceSection } from './maintenance-section';
import { QueueService, DeleteReaperSchedule } from '../services/queue.service';

const makeQueueService = (opts: {
  expiryResult?: { expired: number } | 'error';
  deleteResult?: { deleted: number } | 'error';
  scheduleResult?: DeleteReaperSchedule | 'error';
  updateScheduleResult?: DeleteReaperSchedule | 'error';
} = {}) => {
  const {
    expiryResult = { expired: 0 },
    deleteResult = { deleted: 0 },
    scheduleResult = { schedule: '', active: false },
    updateScheduleResult = { schedule: '', active: false },
  } = opts;

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
    getDeleteReaperSchedule: vi.fn().mockReturnValue(
      scheduleResult === 'error'
        ? throwError(() => ({}))
        : of(scheduleResult as DeleteReaperSchedule),
    ),
    updateDeleteReaperSchedule: vi.fn().mockReturnValue(
      updateScheduleResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to save schedule' } }))
        : of(updateScheduleResult as DeleteReaperSchedule),
    ),
  } as unknown as QueueService;
};

const setup = async (opts: {
  expiryResult?: { expired: number } | 'error';
  deleteResult?: { deleted: number } | 'error';
  scheduleResult?: DeleteReaperSchedule | 'error';
  updateScheduleResult?: DeleteReaperSchedule | 'error';
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

  describe('Delete Reaper schedule', () => {
    describe('on load', () => {
      it('should show the "Not configured" badge when schedule is empty', async () => {
        const { el } = await setup({ scheduleResult: { schedule: '', active: false } });
        expect(el.textContent).toContain('Not configured');
      });

      it('should show the "Active" badge when schedule is set', async () => {
        const { el } = await setup({ scheduleResult: { schedule: '0 2 * * *', active: true } });
        expect(el.textContent).toContain('Active');
      });

      it('should pre-fill the input with the current schedule', async () => {
        const { el } = await setup({ scheduleResult: { schedule: '0 2 * * *', active: true } });
        const input = el.querySelector<HTMLInputElement>('input[id="reaperSchedule"]');
        expect(input?.value).toBe('0 2 * * *');
      });
    });

    describe('when Save is clicked and succeeds', () => {
      it('should call updateDeleteReaperSchedule with the input value', async () => {
        const { fixture, el, component, queueService } = await setup({
          updateScheduleResult: { schedule: '0 3 * * *', active: true },
        });

        component.scheduleInput = '0 3 * * *';
        fixture.detectChanges();

        const saveBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Save',
        ) as HTMLButtonElement;
        saveBtn.click();
        await fixture.whenStable();

        expect(queueService.updateDeleteReaperSchedule).toHaveBeenCalledWith('0 3 * * *');
      });

      it('should show the "Schedule saved." success banner', async () => {
        const { fixture, el, component } = await setup({
          updateScheduleResult: { schedule: '0 3 * * *', active: true },
        });

        component.scheduleInput = '0 3 * * *';
        const saveBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Save',
        ) as HTMLButtonElement;
        saveBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('Schedule saved.');
      });

      it('should update the Active badge after saving', async () => {
        const { fixture, el, component } = await setup({
          scheduleResult: { schedule: '', active: false },
          updateScheduleResult: { schedule: '0 3 * * *', active: true },
        });

        component.scheduleInput = '0 3 * * *';
        const saveBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Save',
        ) as HTMLButtonElement;
        saveBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('Active');
      });
    });

    describe('when Save is clicked and fails', () => {
      it('should show the error message', async () => {
        const { fixture, el, component } = await setup({ updateScheduleResult: 'error' });

        component.scheduleInput = 'bad-cron';
        const saveBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Save',
        ) as HTMLButtonElement;
        saveBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('Failed to save schedule');
      });

      it('should not show the success banner on error', async () => {
        const { fixture, el, component } = await setup({ updateScheduleResult: 'error' });

        component.scheduleInput = 'bad-cron';
        const saveBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Save',
        ) as HTMLButtonElement;
        saveBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).not.toContain('Schedule saved.');
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
