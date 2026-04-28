import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { TopicSchemaSection } from './topic-schema-section';
import { QueueService, TopicSchema } from '../services/queue.service';

const makeSchema = (overrides: Partial<TopicSchema> = {}): TopicSchema => ({
  topic: 'orders',
  schema_json: '{"type":"record","name":"Order","fields":[]}',
  version: 1,
  updated_at: '2024-01-15T10:30:00Z',
  ...overrides,
});

const makeQueueService = (opts: {
  getTopicSchemas?: TopicSchema[] | 'error';
  upsertResult?: TopicSchema | 'error';
  deleteResult?: 'ok' | 'error';
} = {}) => {
  const { getTopicSchemas = [], upsertResult = makeSchema(), deleteResult = 'ok' } = opts;

  return {
    getTopicSchemas: vi.fn().mockReturnValue(
      getTopicSchemas === 'error'
        ? throwError(() => ({ error: { error: 'Failed to load schemas' } }))
        : of(getTopicSchemas as TopicSchema[]),
    ),
    upsertTopicSchema: vi.fn().mockReturnValue(
      upsertResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to save schema' } }))
        : of(upsertResult as TopicSchema),
    ),
    deleteTopicSchema: vi.fn().mockReturnValue(
      deleteResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to delete schema' } }))
        : of(undefined),
    ),
  } as unknown as QueueService;
};

const setup = async (opts: {
  schemas?: TopicSchema[];
  loadError?: boolean;
  upsertResult?: TopicSchema | 'error';
  deleteResult?: 'ok' | 'error';
} = {}) => {
  const { schemas = [], loadError = false, upsertResult, deleteResult } = opts;

  const queueService = makeQueueService({
    getTopicSchemas: loadError ? 'error' : schemas,
    upsertResult: upsertResult ?? makeSchema(),
    deleteResult: deleteResult ?? 'ok',
  });

  await TestBed.configureTestingModule({
    imports: [TopicSchemaSection],
    providers: [
      provideZonelessChangeDetection(),
      { provide: QueueService, useValue: queueService },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(TopicSchemaSection);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  const el: HTMLElement = fixture.nativeElement;
  return { fixture, component: fixture.componentInstance, queueService, el };
};

describe('TopicSchemaSection', () => {
  describe('on init', () => {
    it('should call getTopicSchemas()', async () => {
      const { queueService } = await setup();
      expect(queueService.getTopicSchemas).toHaveBeenCalledOnce();
    });
  });

  describe('when schemas list is empty', () => {
    it('should show empty state message', async () => {
      const { el } = await setup({ schemas: [] });
      expect(el.textContent).toContain('No schemas registered.');
    });

    it('should not render a table', async () => {
      const { el } = await setup({ schemas: [] });
      expect(el.querySelector('table')).toBeNull();
    });
  });

  describe('when schemas are returned', () => {
    it('should render a row per schema', async () => {
      const schemas = [
        makeSchema({ topic: 'orders' }),
        makeSchema({ topic: 'events' }),
        makeSchema({ topic: 'payments' }),
      ];
      const { el } = await setup({ schemas });

      const rows = el.querySelectorAll('tbody tr');
      expect(rows.length).toBe(3);
    });

    it('should show version number', async () => {
      const schemas = [makeSchema({ topic: 'orders', version: 7 })];
      const { el } = await setup({ schemas });

      expect(el.textContent).toContain('7');
    });

    it('should show the topic name in each row', async () => {
      const schemas = [makeSchema({ topic: 'orders' }), makeSchema({ topic: 'events' })];
      const { el } = await setup({ schemas });

      expect(el.textContent).toContain('orders');
      expect(el.textContent).toContain('events');
    });
  });

  describe('when the load fails', () => {
    it('should show an error message', async () => {
      const { el } = await setup({ loadError: true });
      expect(el.textContent).toContain('Failed to load schemas');
    });
  });

  describe('onEdit', () => {
    it('should set editingTopic and populate editForm', async () => {
      const schema = makeSchema({ topic: 'orders', schema_json: '{"type":"record"}' });
      const { fixture, component, el } = await setup({ schemas: [schema] });

      const editBtn = el.querySelector('button[aria-label="Edit schema for orders"]') as HTMLButtonElement;
      expect(editBtn).not.toBeNull();
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.editingTopic()).toBe('orders');
      expect(component.editForm().schema_json).toBe('{"type":"record"}');
    });

    it('should show a textarea in edit mode', async () => {
      const schemas = [makeSchema({ topic: 'orders' })];
      const { fixture, el } = await setup({ schemas });

      const editBtn = el.querySelector('button[aria-label="Edit schema for orders"]') as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const textarea = el.querySelector('textarea[aria-label="Schema JSON for orders"]');
      expect(textarea).not.toBeNull();
    });

    it('should revert to display mode after Cancel without calling the service', async () => {
      const schemas = [makeSchema({ topic: 'orders' })];
      const { fixture, el, queueService } = await setup({ schemas });

      const editBtn = el.querySelector('button[aria-label="Edit schema for orders"]') as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const cancelBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Cancel',
      ) as HTMLButtonElement;
      cancelBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(queueService.upsertTopicSchema).not.toHaveBeenCalled();
      expect(el.querySelector('button[aria-label="Edit schema for orders"]')).not.toBeNull();
    });

    it('should call upsertTopicSchema and update the row after Save', async () => {
      const schemas = [makeSchema({ topic: 'orders', schema_json: '{"type":"record"}', version: 1 })];
      const updated = makeSchema({ topic: 'orders', schema_json: '{"type":"record","name":"Order"}', version: 2 });
      const { fixture, el, component, queueService } = await setup({ schemas, upsertResult: updated });

      const editBtn = el.querySelector('button[aria-label="Edit schema for orders"]') as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.patchEditForm('{"type":"record","name":"Order"}');
      fixture.detectChanges();

      const saveBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
      saveBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(queueService.upsertTopicSchema).toHaveBeenCalledWith('orders', '{"type":"record","name":"Order"}');
      expect(component.editingTopic()).toBeNull();
      expect(component.schemas()[0].version).toBe(2);
    });
  });

  describe('onDelete', () => {
    it('should call deleteTopicSchema and reload', async () => {
      const schemas = [makeSchema({ topic: 'orders' })];
      const { fixture, el, queueService } = await setup({ schemas });

      const deleteBtn = el.querySelector('button[aria-label="Delete schema for orders"]') as HTMLButtonElement;
      expect(deleteBtn).not.toBeNull();
      deleteBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(queueService.deleteTopicSchema).toHaveBeenCalledWith('orders');
      const rows = el.querySelectorAll('tbody tr');
      expect(rows.length).toBe(0);
    });

    it('should remove the row from the DOM after a successful delete', async () => {
      const schemas = [makeSchema({ topic: 'orders' }), makeSchema({ topic: 'events' })];
      const { fixture, el } = await setup({ schemas });

      const deleteBtn = el.querySelector('button[aria-label="Delete schema for orders"]') as HTMLButtonElement;
      deleteBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).not.toContain('orders');
      const rows = el.querySelectorAll('tbody tr');
      expect(rows.length).toBe(1);
    });

    it('should show an error message when delete fails', async () => {
      const schemas = [makeSchema({ topic: 'orders' })];
      const { fixture, el } = await setup({ schemas, deleteResult: 'error' });

      const deleteBtn = el.querySelector('button[aria-label="Delete schema for orders"]') as HTMLButtonElement;
      deleteBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).toContain('Failed to delete schema');
    });
  });

  describe('Add Schema button', () => {
    it('should show a new row with topic input and schema textarea', async () => {
      const { fixture, el, component } = await setup({ schemas: [] });

      const addBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim().includes('Add Schema'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.addingNew()).toBe(true);
      expect(el.querySelector('input[aria-label="New topic name"]')).not.toBeNull();
      expect(el.querySelector('textarea[aria-label="New schema JSON"]')).not.toBeNull();
    });

    it('should cancel the new row without adding a schema', async () => {
      const { fixture, el, component } = await setup({ schemas: [] });

      const addBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim().includes('Add Schema'),
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

      expect(component.addingNew()).toBe(false);
      expect(component.schemas().length).toBe(0);
    });

    it('should show an error when topic name is empty on save', async () => {
      const { fixture, el, component } = await setup({ schemas: [] });

      const addBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim().includes('Add Schema'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newTopic.set('');

      const saveBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
      saveBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(el.textContent).toContain('Topic name is required');
    });
  });
});
