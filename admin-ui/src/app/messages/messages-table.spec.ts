import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { By } from '@angular/platform-browser';
import { CdkVirtualScrollViewport } from '@angular/cdk/scrolling';
import { MessagesTable } from './messages-table';
import { QueueMessage } from '../services/queue.service';

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

const setup = async (opts: {
  messages?: QueueMessage[];
  loading?: boolean;
  error?: string;
} = {}) => {
  const { messages = [], loading = false, error = '' } = opts;

  await TestBed.configureTestingModule({
    imports: [MessagesTable],
    providers: [provideZonelessChangeDetection()],
  }).compileComponents();

  const fixture = TestBed.createComponent(MessagesTable);
  fixture.componentRef.setInput('messages', messages);
  fixture.componentRef.setInput('loading', loading);
  fixture.componentRef.setInput('error', error);
  fixture.detectChanges();
  await fixture.whenStable();

  const vpQuery = fixture.debugElement.query(By.directive(CdkVirtualScrollViewport));
  if (vpQuery) {
    Object.defineProperty(vpQuery.nativeElement, 'clientHeight', { value: 520, configurable: true });
    vpQuery.injector.get(CdkVirtualScrollViewport).checkViewportSize();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  return { fixture, component: fixture.componentInstance };
};

describe('MessagesTable', () => {
  describe('statusClasses()', () => {
    let component: MessagesTable;

    beforeEach(async () => {
      const result = await setup();
      component = result.component;
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
        expect(component.statusClasses('other')).toContain('bg-gray-100');
      });
    });
  });

  describe('when the Refresh button is clicked', () => {
    it('should emit a topicSearch event', async () => {
      const { fixture } = await setup({ messages: [] });

      const emitted: string[] = [];
      fixture.componentInstance.topicSearch.subscribe((v: string) => emitted.push(v));

      const el: HTMLElement = fixture.nativeElement;
      const refreshButton = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Refresh'),
      ) as HTMLButtonElement;
      refreshButton.click();
      await fixture.whenStable();

      expect(emitted.length).toBe(1);
    });
  });

  describe('when messages is empty and loading is false', () => {
    it('should show "No messages found"', async () => {
      const { fixture } = await setup({ messages: [], loading: false });

      const el: HTMLElement = fixture.nativeElement;
      expect(el.textContent).toContain('No messages found');
    });
  });

  describe('when messages are provided', () => {
    it('should render one tbody tr per message', async () => {
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

  describe('Purge button', () => {
    const typeInFilter = async (
      fixture: Awaited<ReturnType<typeof setup>>['fixture'],
      value: string,
    ) => {
      const el: HTMLElement = fixture.nativeElement;
      const input = el.querySelector('input[type="text"]') as HTMLInputElement;
      input.value = value;
      input.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.detectChanges();
    };

    describe('when no topic filter is active', () => {
      it('should not show the Purge button', async () => {
        const { fixture } = await setup({ messages: [] });

        const el: HTMLElement = fixture.nativeElement;
        const purgeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Purge',
        );
        expect(purgeBtn).toBeUndefined();
      });
    });

    describe('when a topic filter is active', () => {
      it('should show the Purge button', async () => {
        const { fixture } = await setup({ messages: [] });
        await typeInFilter(fixture, 'orders');

        const el: HTMLElement = fixture.nativeElement;
        const purgeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Purge',
        );
        expect(purgeBtn).not.toBeUndefined();
      });

      it('should show the confirm panel when Purge is clicked', async () => {
        const { fixture } = await setup({ messages: [] });
        await typeInFilter(fixture, 'orders');

        const el: HTMLElement = fixture.nativeElement;
        const purgeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Purge',
        ) as HTMLButtonElement;
        purgeBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(el.textContent).toContain('Purge messages from "orders"');
        const confirmBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Confirm purge',
        );
        expect(confirmBtn).not.toBeUndefined();
      });

      it('should hide the confirm panel when Cancel is clicked', async () => {
        const { fixture } = await setup({ messages: [] });
        await typeInFilter(fixture, 'orders');

        const el: HTMLElement = fixture.nativeElement;
        const purgeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Purge',
        ) as HTMLButtonElement;
        purgeBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        const cancelBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Cancel',
        ) as HTMLButtonElement;
        cancelBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        const confirmBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Confirm purge',
        );
        expect(confirmBtn).toBeUndefined();
      });

      it('should emit purge output with the correct topic and statuses when confirmed', async () => {
        const { fixture } = await setup({ messages: [] });
        await typeInFilter(fixture, 'orders');

        const el: HTMLElement = fixture.nativeElement;
        const emitted: { topic: string; statuses: string[] }[] = [];
        fixture.componentInstance.purge.subscribe((v: { topic: string; statuses: string[] }) =>
          emitted.push(v),
        );

        const purgeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Purge',
        ) as HTMLButtonElement;
        purgeBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        const confirmBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Confirm purge',
        ) as HTMLButtonElement;
        confirmBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(emitted.length).toBe(1);
        expect(emitted[0].topic).toBe('orders');
        expect(emitted[0].statuses).toEqual(['pending', 'processing', 'expired']);
      });

      it('should close the confirm panel after confirming', async () => {
        const { fixture } = await setup({ messages: [] });
        await typeInFilter(fixture, 'orders');

        const el: HTMLElement = fixture.nativeElement;
        const purgeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Purge',
        ) as HTMLButtonElement;
        purgeBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        const confirmBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Confirm purge',
        ) as HTMLButtonElement;
        confirmBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(
          Array.from(el.querySelectorAll('button')).find((b) =>
            b.textContent?.trim() === 'Confirm purge',
          ),
        ).toBeUndefined();
      });

      it('should emit only the checked statuses when a status is unchecked', async () => {
        const { fixture, component } = await setup({ messages: [] });
        await typeInFilter(fixture, 'orders');

        // Uncheck 'expired'
        component.togglePurgeStatus('expired');

        const el: HTMLElement = fixture.nativeElement;
        const emitted: { topic: string; statuses: string[] }[] = [];
        fixture.componentInstance.purge.subscribe((v: { topic: string; statuses: string[] }) =>
          emitted.push(v),
        );

        const purgeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Purge',
        ) as HTMLButtonElement;
        purgeBtn.click();
        await fixture.whenStable();
        fixture.detectChanges();

        const confirmBtn = Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.trim() === 'Confirm purge',
        ) as HTMLButtonElement;
        confirmBtn.click();
        await fixture.whenStable();

        expect(emitted[0].statuses).toEqual(['pending', 'processing']);
      });
    });
  });
});
