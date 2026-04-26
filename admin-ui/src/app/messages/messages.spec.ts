import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { provideRouter } from '@angular/router';
import { Router } from '@angular/router';
import { of, throwError } from 'rxjs';
import { Messages } from './messages';
import { AuthService } from '../services/auth.service';
import { QueueService, QueueMessage } from '../services/queue.service';

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

const makeAuthService = (authenticated = false) =>
  ({
    isAuthenticated: vi.fn().mockReturnValue(authenticated),
    logout: vi.fn(),
    getAuthHeader: () => null,
  }) as unknown as AuthService;

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
        : of(opts.listResult),
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

  describe('statusClasses()', () => {
    let component: Messages;

    beforeEach(async () => {
      const { fixture } = await setup();
      component = fixture.componentInstance;
    });

    describe('when status is "pending"', () => {
      it('should return a string containing bg-yellow-100', () => {
        expect(component.statusClasses('pending')).toContain('bg-yellow-100');
      });
    });

    describe('when status is "processing"', () => {
      it('should return a string containing bg-blue-100', () => {
        expect(component.statusClasses('processing')).toContain('bg-blue-100');
      });
    });

    describe('when status is "failed"', () => {
      it('should return a string containing bg-red-100', () => {
        expect(component.statusClasses('failed')).toContain('bg-red-100');
      });
    });

    describe('when status is "expired"', () => {
      it('should return a string containing bg-orange-100', () => {
        expect(component.statusClasses('expired')).toContain('bg-orange-100');
      });
    });

    describe('when status is an unknown value', () => {
      it('should return a string containing bg-gray-100', () => {
        expect(component.statusClasses('completed')).toContain('bg-gray-100');
        expect(component.statusClasses('other')).toContain('bg-gray-100');
      });
    });
  });

  describe('addMetadataRow()', () => {
    it('should push a new entry onto metadataRows signal', async () => {
      const { fixture } = await setup();
      const component = fixture.componentInstance;

      expect(component.metadataRows().length).toBe(0);
      // form() inside addMetadataRow() calls inject(), so run it in injection context
      TestBed.runInInjectionContext(() => component.addMetadataRow());
      expect(component.metadataRows().length).toBe(1);
      TestBed.runInInjectionContext(() => component.addMetadataRow());
      expect(component.metadataRows().length).toBe(2);
    });
  });

  describe('removeMetadataRow()', () => {
    it('should remove the entry at the given index', async () => {
      const { fixture } = await setup();
      const component = fixture.componentInstance;

      TestBed.runInInjectionContext(() => component.addMetadataRow());
      TestBed.runInInjectionContext(() => component.addMetadataRow());
      expect(component.metadataRows().length).toBe(2);

      component.removeMetadataRow(0);
      expect(component.metadataRows().length).toBe(1);
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
      // The enqueue form is the last form on the page
      const forms = el.querySelectorAll('form');
      const enqueueForm = forms[forms.length - 1];
      enqueueForm.dispatchEvent(new Event('submit'));
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).toContain('enqueued-id');
    });
  });

  describe('on enqueue error', () => {
    it('should show an error banner', async () => {
      const { fixture } = await setup({ enqueueError: true });

      const el: HTMLElement = fixture.nativeElement;
      const forms = el.querySelectorAll('form');
      const enqueueForm = forms[forms.length - 1];
      enqueueForm.dispatchEvent(new Event('submit'));
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
