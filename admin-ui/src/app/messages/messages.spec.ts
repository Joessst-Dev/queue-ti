import { vi } from 'vitest';

vi.mock('chart.js', () => ({
  Chart: class {
    static register = vi.fn();
    update = vi.fn();
    destroy = vi.fn();
  },
  BarController: {},
  BarElement: {},
  CategoryScale: {},
  LinearScale: {},
  Tooltip: {},
  Legend: {},
}));

import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { provideRouter } from '@angular/router';
import { Router } from '@angular/router';
import { By } from '@angular/platform-browser';
import { of, throwError } from 'rxjs';
import { CdkVirtualScrollViewport } from '@angular/cdk/scrolling';
import { Messages } from './messages';
import { AuthService } from '../services/auth.service';
import { QueueService, QueueMessage, StatsResponse } from '../services/queue.service';

const makeMessage = (overrides: Partial<QueueMessage> = {}): QueueMessage => ({
  id: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
  topic: 'orders',
  payload: '{"orderId": 1}',
  metadata: {},
  status: 'pending',
  created_at: '2024-01-15T10:30:00Z',
  retry_count: 0,
  max_retries: 3,
  last_error: '',
  expires_at: null,
  original_topic: null,
  dlq_moved_at: null,
  ...overrides,
});

const makeAuthService = (authenticated = false, isAdmin = false) =>
  ({
    isAuthenticated: vi.fn().mockReturnValue(authenticated),
    isAdmin: vi.fn().mockReturnValue(isAdmin),
    logout: vi.fn(),
    getAuthHeader: () => null,
  }) as unknown as AuthService;

const emptyStats: StatsResponse = { topics: [] };

const makeQueueService = (opts: {
  listResult: QueueMessage[] | 'error';
  enqueueResult: { id: string } | 'error';
  nackResult?: 'error';
  requeueResult?: 'error';
}) =>
  ({
    listMessages: vi.fn().mockReturnValue(
      opts.listResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to load messages' } }))
        : of({
            items: opts.listResult as QueueMessage[],
            total: (opts.listResult as QueueMessage[]).length,
            limit: 50,
            offset: 0,
          }),
    ),
    enqueueMessage: vi.fn().mockReturnValue(
      opts.enqueueResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to enqueue message' } }))
        : of(opts.enqueueResult),
    ),
    nackMessage: vi.fn().mockReturnValue(
      opts.nackResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to nack' } }))
        : of(undefined),
    ),
    requeueMessage: vi.fn().mockReturnValue(
      opts.requeueResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to requeue' } }))
        : of(undefined),
    ),
    getStats: vi.fn().mockReturnValue(of(emptyStats)),
  }) as unknown as QueueService;

const setup = async (opts: {
  messages?: QueueMessage[];
  listError?: boolean;
  enqueueError?: boolean;
  authenticated?: boolean;
} = {}) => {
  const {
    messages = [],
    listError = false,
    enqueueError = false,
    authenticated = true,
  } = opts;

  const authService = makeAuthService(authenticated);
  const queueService = makeQueueService({
    listResult: listError ? 'error' : messages,
    enqueueResult: enqueueError ? 'error' : { id: 'enqueued-id' },
  });

  await TestBed.configureTestingModule({
    imports: [Messages],
    providers: [
      provideZonelessChangeDetection(),
      provideRouter([]),
      { provide: AuthService, useValue: authService },
      { provide: QueueService, useValue: queueService },
    ],
  }).compileComponents();

  const router = TestBed.inject(Router);
  vi.spyOn(router, 'navigate').mockResolvedValue(true);

  const fixture = TestBed.createComponent(Messages);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  // JSDOM reports all elements as having zero dimensions. Give the CDK viewport
  // a non-zero clientHeight so it renders items instead of the empty virtual range.
  const vpQuery = fixture.debugElement.query(By.directive(CdkVirtualScrollViewport));
  if (vpQuery) {
    Object.defineProperty(vpQuery.nativeElement, 'clientHeight', { value: 520, configurable: true });
    vpQuery.injector.get(CdkVirtualScrollViewport).checkViewportSize();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  return { fixture, authService, queueService, router };
};

describe('Messages', () => {
  describe('on component initialisation', () => {
    it('should trigger listMessages automatically', async () => {
      const { queueService } = await setup();
      expect(queueService.listMessages).toHaveBeenCalled();
    });
  });

  describe('when messages are returned by the API', () => {
    it('should display one table row per message', async () => {
      const messages = [
        makeMessage({ id: 'aaaaaaaa-1111-cccc-dddd-eeeeeeeeeeee' }),
        makeMessage({ id: 'aaaaaaaa-2222-cccc-dddd-eeeeeeeeeeee' }),
        makeMessage({ id: 'aaaaaaaa-3333-cccc-dddd-eeeeeeeeeeee' }),
      ];
      const { fixture } = await setup({ messages });

      const el: HTMLElement = fixture.nativeElement;
      const rows = el.querySelectorAll('tbody tr');
      expect(rows.length).toBe(3);
    });
  });

  describe('when the message list is empty and not loading', () => {
    it('should show "No messages found" empty state', async () => {
      const { fixture } = await setup({ messages: [] });

      const el: HTMLElement = fixture.nativeElement;
      expect(el.textContent).toContain('No messages found');
    });
  });

  describe('when "Sign out" is clicked', () => {
    it('should call auth.logout()', async () => {
      const { fixture, authService } = await setup({ authenticated: true });

      const el: HTMLElement = fixture.nativeElement;
      const signOutButton = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Sign out'),
      );
      expect(signOutButton).not.toBeUndefined();
      signOutButton?.click();

      expect(authService.logout).toHaveBeenCalledOnce();
    });

    it('should navigate to /login', async () => {
      const { fixture, router } = await setup({ authenticated: true });

      const el: HTMLElement = fixture.nativeElement;
      const signOutButton = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Sign out'),
      );
      signOutButton?.click();

      expect(router.navigate).toHaveBeenCalledWith(['/login']);
    });
  });

  describe('on enqueue success', () => {
    it('should show a success banner with the returned ID', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      // Switch to the Enqueue tab first
      const enqueueTab = Array.from(el.querySelectorAll('nav button')).find((b) =>
        b.textContent?.trim() === 'Enqueue',
      ) as HTMLButtonElement;
      enqueueTab.click();
      fixture.detectChanges();

      const form = el.querySelector('form') as HTMLFormElement;
      form.dispatchEvent(new Event('submit'));
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).toContain('enqueued-id');
    });
  });

  describe('on enqueue error', () => {
    it('should show an error banner', async () => {
      const { fixture } = await setup({ enqueueError: true });
      const el: HTMLElement = fixture.nativeElement;

      const enqueueTab = Array.from(el.querySelectorAll('nav button')).find((b) =>
        b.textContent?.trim() === 'Enqueue',
      ) as HTMLButtonElement;
      enqueueTab.click();
      fixture.detectChanges();

      const form = el.querySelector('form') as HTMLFormElement;
      form.dispatchEvent(new Event('submit'));
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).toContain('Failed to enqueue message');
    });
  });

  describe('when a DLQ message is displayed (topic ends with .dlq)', () => {
    it('should show a Requeue button for the DLQ message', async () => {
      const dlqMsg = makeMessage({ topic: 'orders.dlq', status: 'failed', original_topic: 'orders' });
      const { fixture } = await setup({ messages: [dlqMsg] });

      const el: HTMLElement = fixture.nativeElement;
      const requeueButton = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Requeue'),
      );
      expect(requeueButton).not.toBeUndefined();
    });

    it('should NOT show a Requeue button for a regular message', async () => {
      const regularMsg = makeMessage({ topic: 'orders', status: 'pending' });
      const { fixture } = await setup({ messages: [regularMsg] });

      const el: HTMLElement = fixture.nativeElement;
      const requeueButton = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Requeue'),
      );
      expect(requeueButton).toBeUndefined();
    });

    it('should call queue.requeueMessage and refresh when Requeue is clicked', async () => {
      const dlqMsg = makeMessage({ topic: 'orders.dlq', status: 'failed', original_topic: 'orders' });
      const { fixture, queueService } = await setup({ messages: [dlqMsg] });

      const el: HTMLElement = fixture.nativeElement;
      const requeueButton = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Requeue'),
      ) as HTMLButtonElement;

      requeueButton.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(queueService.requeueMessage).toHaveBeenCalledWith(dlqMsg.id);
      expect(queueService.listMessages).toHaveBeenCalledTimes(2);
    });
  });
});
