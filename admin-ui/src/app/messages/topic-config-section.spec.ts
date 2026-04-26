import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { TopicConfigSection } from './topic-config-section';
import { QueueService, TopicConfig, TopicConfigsResponse } from '../services/queue.service';

const makeConfig = (overrides: Partial<TopicConfig> = {}): TopicConfig => ({
  topic: 'orders',
  max_retries: 3,
  message_ttl_seconds: 3600,
  max_depth: null,
  ...overrides,
});

const makeQueueService = (opts: {
  getTopicConfigs?: TopicConfig[] | 'error';
  upsertResult?: TopicConfig | 'error';
  deleteResult?: 'ok' | 'error';
} = {}) => {
  const { getTopicConfigs = [], upsertResult = makeConfig(), deleteResult = 'ok' } = opts;

  const response: TopicConfigsResponse = {
    items: getTopicConfigs === 'error' ? [] : (getTopicConfigs as TopicConfig[]),
  };

  return {
    getTopicConfigs: vi.fn().mockReturnValue(
      getTopicConfigs === 'error'
        ? throwError(() => ({ error: { error: 'Failed to load topic configs' } }))
        : of(response),
    ),
    upsertTopicConfig: vi.fn().mockReturnValue(
      upsertResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to save topic config' } }))
        : of(upsertResult as TopicConfig),
    ),
    deleteTopicConfig: vi.fn().mockReturnValue(
      deleteResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to delete topic config' } }))
        : of(undefined),
    ),
  } as unknown as QueueService;
};

const setup = async (opts: {
  configs?: TopicConfig[];
  loadError?: boolean;
  upsertResult?: TopicConfig | 'error';
  deleteResult?: 'ok' | 'error';
} = {}) => {
  const { configs = [], loadError = false, upsertResult, deleteResult } = opts;

  const queueService = makeQueueService({
    getTopicConfigs: loadError ? 'error' : configs,
    upsertResult: upsertResult ?? makeConfig(),
    deleteResult: deleteResult ?? 'ok',
  });

  await TestBed.configureTestingModule({
    imports: [TopicConfigSection],
    providers: [
      provideZonelessChangeDetection(),
      { provide: QueueService, useValue: queueService },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(TopicConfigSection);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  const el: HTMLElement = fixture.nativeElement;
  return { fixture, component: fixture.componentInstance, queueService, el };
};

describe('TopicConfigSection', () => {
  describe('on init', () => {
    it('should call getTopicConfigs() immediately', async () => {
      const { queueService } = await setup();
      expect(queueService.getTopicConfigs).toHaveBeenCalledOnce();
    });
  });

  describe('when configs are empty', () => {
    it('should show the empty-state message', async () => {
      const { el } = await setup({ configs: [] });
      expect(el.textContent).toContain('No topic configurations');
    });

    it('should not render a table', async () => {
      const { el } = await setup({ configs: [] });
      expect(el.querySelector('table')).toBeNull();
    });
  });

  describe('when configs are returned', () => {
    it('should render one table row per config', async () => {
      const configs = [
        makeConfig({ topic: 'orders' }),
        makeConfig({ topic: 'events' }),
        makeConfig({ topic: 'notifications' }),
      ];
      const { el } = await setup({ configs });

      const rows = el.querySelectorAll('tbody tr');
      expect(rows.length).toBe(3);
    });

    it('should show — for null fields', async () => {
      const configs = [makeConfig({ topic: 'orders', max_retries: null, message_ttl_seconds: null, max_depth: null })];
      const { el } = await setup({ configs });

      const dashes = el.querySelectorAll('span.text-gray-400');
      // three nullable columns, all null
      expect(dashes.length).toBe(3);
      dashes.forEach((d) => expect(d.textContent?.trim()).toBe('—'));
    });

    it('should display numeric values when set', async () => {
      const configs = [makeConfig({ topic: 'orders', max_retries: 5, message_ttl_seconds: 1800, max_depth: 200 })];
      const { el } = await setup({ configs });

      expect(el.textContent).toContain('5');
      expect(el.textContent).toContain('1800');
      expect(el.textContent).toContain('200');
    });
  });

  describe('when the load fails', () => {
    it('should show an error message', async () => {
      const { el } = await setup({ loadError: true });
      expect(el.textContent).toContain('Failed to load topic configs');
    });
  });

  describe('Delete button', () => {
    it('should call deleteTopicConfig with the topic when clicked', async () => {
      const configs = [makeConfig({ topic: 'orders' })];
      const { el, queueService } = await setup({ configs });

      const deleteBtn = el.querySelector('button[aria-label="Delete orders"]') as HTMLButtonElement;
      expect(deleteBtn).not.toBeNull();
      deleteBtn.click();

      expect(queueService.deleteTopicConfig).toHaveBeenCalledWith('orders');
    });

    it('should remove the row from the DOM after a successful delete', async () => {
      const configs = [makeConfig({ topic: 'orders' }), makeConfig({ topic: 'events' })];
      const { fixture, el, queueService } = await setup({ configs });

      const deleteBtn = el.querySelector('button[aria-label="Delete orders"]') as HTMLButtonElement;
      deleteBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(queueService.deleteTopicConfig).toHaveBeenCalledWith('orders');
      const rows = el.querySelectorAll('tbody tr');
      expect(rows.length).toBe(1);
      expect(el.textContent).not.toContain('orders');
    });

    it('should show an error message when delete fails', async () => {
      const configs = [makeConfig({ topic: 'orders' })];
      const { fixture, el } = await setup({ configs, deleteResult: 'error' });

      const deleteBtn = el.querySelector('button[aria-label="Delete orders"]') as HTMLButtonElement;
      deleteBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).toContain('Failed to delete topic config');
    });
  });

  describe('Edit button', () => {
    it('should switch the row to edit mode showing inputs', async () => {
      const configs = [makeConfig({ topic: 'orders', max_retries: 3 })];
      const { fixture, el } = await setup({ configs });

      const editBtn = el.querySelector('button[aria-label="Edit orders"]') as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const inputs = el.querySelectorAll('input[type="number"]');
      expect(inputs.length).toBeGreaterThanOrEqual(3);
    });

    it('should pre-populate the edit inputs with the current values', async () => {
      const configs = [makeConfig({ topic: 'orders', max_retries: 7, message_ttl_seconds: 900, max_depth: null })];
      const { fixture, el } = await setup({ configs });

      const editBtn = el.querySelector('button[aria-label="Edit orders"]') as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const { component } = { component: fixture.componentInstance };
      expect(component.editForm().max_retries).toBe('7');
      expect(component.editForm().message_ttl_seconds).toBe('900');
      expect(component.editForm().max_depth).toBe('');
    });

    it('should revert to display mode after Cancel without calling the service', async () => {
      const configs = [makeConfig({ topic: 'orders' })];
      const { fixture, el, queueService } = await setup({ configs });

      const editBtn = el.querySelector('button[aria-label="Edit orders"]') as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const cancelBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Cancel',
      ) as HTMLButtonElement;
      expect(cancelBtn).not.toBeUndefined();
      cancelBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(queueService.upsertTopicConfig).not.toHaveBeenCalled();
      // back to display mode: Edit button is visible again
      expect(el.querySelector('button[aria-label="Edit orders"]')).not.toBeNull();
    });

    it('should call upsertTopicConfig and update the row after Save', async () => {
      const configs = [makeConfig({ topic: 'orders', max_retries: 3, message_ttl_seconds: 3600, max_depth: null })];
      const updated = makeConfig({ topic: 'orders', max_retries: 5, message_ttl_seconds: 3600, max_depth: null });
      const { fixture, el, queueService, component } = await setup({ configs, upsertResult: updated });

      const editBtn = el.querySelector('button[aria-label="Edit orders"]') as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.patchEditForm('max_retries', '5');
      fixture.detectChanges();

      const saveBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
      saveBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(queueService.upsertTopicConfig).toHaveBeenCalledWith('orders', expect.objectContaining({ max_retries: 5 }));
      // back to display mode
      expect(el.querySelector('button[aria-label="Edit orders"]')).not.toBeNull();
    });
  });

  describe('Add config button', () => {
    it('should show a new editable row at the top of the table', async () => {
      const { fixture, el, component } = await setup({ configs: [] });

      const addBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim().includes('Add config'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.editingTopic()).toBe('__new__');
      const topicInput = el.querySelector('input[aria-label="New topic name"]');
      expect(topicInput).not.toBeNull();
    });

    it('should cancel the new row without adding a config', async () => {
      const { fixture, el, component } = await setup({ configs: [] });

      const addBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim().includes('Add config'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const cancelBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Cancel',
      ) as HTMLButtonElement;
      cancelBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.editingTopic()).toBeNull();
      expect(component.configs().length).toBe(0);
    });
  });
});
