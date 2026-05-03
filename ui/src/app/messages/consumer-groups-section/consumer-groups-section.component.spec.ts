import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { ConsumerGroupsSection } from './consumer-groups-section.component';
import { QueueService, ConsumerGroupsResponse } from '../../services/queue.service';
import { HttpErrorResponse } from '@angular/common/http';

const makeQueueService = (opts: {
  listResult?: string[] | 'error';
  registerResult?: 'ok' | 'conflict' | 'error';
  unregisterResult?: 'ok' | 'error';
} = {}): QueueService => {
  const { listResult = [], registerResult = 'ok', unregisterResult = 'ok' } = opts;

  const listResponse: ConsumerGroupsResponse = {
    items: listResult === 'error' ? [] : (listResult as string[]),
  };

  const listObservable =
    listResult === 'error'
      ? throwError(() => new HttpErrorResponse({ error: { error: 'Failed to load consumer groups' }, status: 500 }))
      : of(listResponse);

  const registerObservable = (() => {
    if (registerResult === 'conflict') {
      return throwError(() => new HttpErrorResponse({ status: 409 }));
    }
    if (registerResult === 'error') {
      return throwError(() => new HttpErrorResponse({ error: { error: 'Failed to register consumer group' }, status: 500 }));
    }
    return of(undefined as void);
  })();

  const unregisterObservable =
    unregisterResult === 'error'
      ? throwError(() => new HttpErrorResponse({ error: { error: 'Failed to unregister consumer group' }, status: 500 }))
      : of(undefined as void);

  return {
    listConsumerGroups: vi.fn().mockReturnValue(listObservable),
    registerConsumerGroup: vi.fn().mockReturnValue(registerObservable),
    unregisterConsumerGroup: vi.fn().mockReturnValue(unregisterObservable),
  } as unknown as QueueService;
};

const setup = async (opts: {
  topic?: string;
  groups?: string[] | 'error';
  registerResult?: 'ok' | 'conflict' | 'error';
  unregisterResult?: 'ok' | 'error';
} = {}) => {
  const {
    topic = 'orders',
    groups = [],
    registerResult = 'ok',
    unregisterResult = 'ok',
  } = opts;

  const queueService = makeQueueService({
    listResult: groups,
    registerResult,
    unregisterResult,
  });

  await TestBed.configureTestingModule({
    imports: [ConsumerGroupsSection],
    providers: [
      provideZonelessChangeDetection(),
      { provide: QueueService, useValue: queueService },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(ConsumerGroupsSection);
  fixture.componentRef.setInput('topic', topic);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  const el: HTMLElement = fixture.nativeElement;
  return { fixture, component: fixture.componentInstance, queueService, el };
};

describe('ConsumerGroupsSection', () => {
  describe('on init', () => {
    it('should call listConsumerGroups with the topic', async () => {
      const { queueService } = await setup({ topic: 'payments' });
      expect(queueService.listConsumerGroups).toHaveBeenCalledWith('payments');
    });
  });

  describe('when there are no consumer groups', () => {
    it('should show the empty-state message', async () => {
      const { el } = await setup({ groups: [] });
      expect(el.textContent).toContain('No consumer groups registered.');
    });

    it('should not render a table', async () => {
      const { el } = await setup({ groups: [] });
      expect(el.querySelector('table')).toBeNull();
    });
  });

  describe('when consumer groups are returned', () => {
    it('should render one row per group', async () => {
      const { el } = await setup({ groups: ['groupA', 'groupB', 'groupC'] });
      const rows = el.querySelectorAll('tbody tr');
      expect(rows.length).toBe(3);
    });

    it('should display each group name', async () => {
      const { el } = await setup({ groups: ['alpha', 'beta'] });
      expect(el.textContent).toContain('alpha');
      expect(el.textContent).toContain('beta');
    });

    it('should render a Delete button per row with an aria-label', async () => {
      const { el } = await setup({ groups: ['groupA'] });
      const btn = el.querySelector('button[aria-label="Delete consumer group groupA"]');
      expect(btn).not.toBeNull();
    });
  });

  describe('when the load fails', () => {
    it('should show an error message', async () => {
      const { el } = await setup({ groups: 'error' });
      expect(el.textContent).toContain('Failed to load consumer groups');
    });
  });

  describe('Register button', () => {
    it('should be disabled when the input is empty', async () => {
      const { el } = await setup();
      const btn = el.querySelector('button[type="submit"]') as HTMLButtonElement;
      expect(btn.disabled).toBe(true);
    });

    it('should be enabled when the input has a non-empty value', async () => {
      const { fixture, el, component } = await setup();
      component.newGroupName.set('my-group');
      fixture.detectChanges();
      const btn = el.querySelector('button[type="submit"]') as HTMLButtonElement;
      expect(btn.disabled).toBe(false);
    });

    it('should call registerConsumerGroup with topic and trimmed name', async () => {
      const { fixture, component, queueService } = await setup({ topic: 'orders' });
      component.newGroupName.set('  service-a  ');
      fixture.detectChanges();
      component.registerGroup();
      await fixture.whenStable();
      expect(queueService.registerConsumerGroup).toHaveBeenCalledWith('orders', 'service-a');
    });

    it('should clear the input and reload after a successful registration', async () => {
      const { fixture, component, queueService } = await setup();
      component.newGroupName.set('my-group');
      component.registerGroup();
      await fixture.whenStable();
      fixture.detectChanges();
      expect(component.newGroupName()).toBe('');
      expect(queueService.listConsumerGroups).toHaveBeenCalledTimes(2);
    });

    it('should not call the service when the name is only whitespace', async () => {
      const { component, queueService } = await setup();
      component.newGroupName.set('   ');
      component.registerGroup();
      expect(queueService.registerConsumerGroup).not.toHaveBeenCalled();
    });

    it('should show a conflict error on 409', async () => {
      const { fixture, el, component } = await setup({ registerResult: 'conflict' });
      component.newGroupName.set('dupe');
      component.registerGroup();
      await fixture.whenStable();
      fixture.detectChanges();
      expect(el.textContent).toContain('already registered');
    });

    it('should show a generic error on non-409 failure', async () => {
      const { fixture, el, component } = await setup({ registerResult: 'error' });
      component.newGroupName.set('bad');
      component.registerGroup();
      await fixture.whenStable();
      fixture.detectChanges();
      expect(el.textContent).toContain('Failed to register consumer group');
    });
  });

  describe('Delete button', () => {
    it('should call unregisterConsumerGroup with topic and group', async () => {
      const { fixture, el, queueService } = await setup({ topic: 'orders', groups: ['groupA'] });
      const btn = el.querySelector('button[aria-label="Delete consumer group groupA"]') as HTMLButtonElement;
      btn.click();
      await fixture.whenStable();
      expect(queueService.unregisterConsumerGroup).toHaveBeenCalledWith('orders', 'groupA');
    });

    it('should reload the list after a successful deletion', async () => {
      const { fixture, el, queueService } = await setup({ groups: ['groupA'] });
      const btn = el.querySelector('button[aria-label="Delete consumer group groupA"]') as HTMLButtonElement;
      btn.click();
      await fixture.whenStable();
      // initial load + reload after delete
      expect(queueService.listConsumerGroups).toHaveBeenCalledTimes(2);
    });

    it('should show an error message when deletion fails', async () => {
      const { fixture, el } = await setup({ groups: ['groupA'], unregisterResult: 'error' });
      const btn = el.querySelector('button[aria-label="Delete consumer group groupA"]') as HTMLButtonElement;
      btn.click();
      await fixture.whenStable();
      fixture.detectChanges();
      expect(el.textContent).toContain('Failed to unregister consumer group');
    });
  });

  describe('section heading', () => {
    it('should display "Consumer Groups"', async () => {
      const { el } = await setup();
      expect(el.textContent).toContain('Consumer Groups');
    });
  });

  describe('register form', () => {
    it('should always render the group name input and Register button', async () => {
      const { el } = await setup();
      expect(el.querySelector('input[aria-label="New consumer group name"]')).not.toBeNull();
      expect(el.querySelector('button[type="submit"]')).not.toBeNull();
    });
  });
});
