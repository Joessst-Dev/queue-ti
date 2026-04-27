import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { EnqueueSection } from './enqueue-section';
import { EnqueueRequest, QueueService, TopicSchema } from '../services/queue.service';

const makeSchema = (overrides: Partial<TopicSchema> = {}): TopicSchema => ({
  topic: 'orders',
  schema_json: '{"type":"record","name":"Order","fields":[{"name":"id","type":"string"}]}',
  version: 1,
  updated_at: '2024-01-15T10:30:00Z',
  ...overrides,
});

const makeQueueService = (opts: {
  getTopicSchemaResult?: TopicSchema | 'error';
} = {}) => ({
  getTopicSchema: vi.fn().mockReturnValue(
    opts.getTopicSchemaResult === 'error'
      ? throwError(() => ({ status: 404 }))
      : of(opts.getTopicSchemaResult ?? makeSchema()),
  ),
}) as unknown as QueueService;

const setup = async (opts: {
  success?: string;
  error?: string;
  loading?: boolean;
  queueService?: QueueService;
} = {}) => {
  const { success = '', error = '', loading = false, queueService = makeQueueService() } = opts;

  await TestBed.configureTestingModule({
    imports: [EnqueueSection],
    providers: [
      provideZonelessChangeDetection(),
      { provide: QueueService, useValue: queueService },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(EnqueueSection);
  fixture.componentRef.setInput('success', success);
  fixture.componentRef.setInput('error', error);
  fixture.componentRef.setInput('loading', loading);
  fixture.detectChanges();
  await fixture.whenStable();

  return { fixture, component: fixture.componentInstance };
};

describe('EnqueueSection', () => {
  describe('addMetadataRow()', () => {
    it('should push a new entry onto metadataRows signal', async () => {
      const { component } = await setup();

      expect(component.metadataRows().length).toBe(0);
      TestBed.runInInjectionContext(() => component.addMetadataRow());
      expect(component.metadataRows().length).toBe(1);
      TestBed.runInInjectionContext(() => component.addMetadataRow());
      expect(component.metadataRows().length).toBe(2);
    });
  });

  describe('removeMetadataRow()', () => {
    it('should remove the entry at the given index', async () => {
      const { component } = await setup();

      TestBed.runInInjectionContext(() => component.addMetadataRow());
      TestBed.runInInjectionContext(() => component.addMetadataRow());
      expect(component.metadataRows().length).toBe(2);

      component.removeMetadataRow(0);
      expect(component.metadataRows().length).toBe(1);
    });
  });

  describe('when the success input changes to a truthy value', () => {
    it('should reset the enqueue form model', async () => {
      const { fixture, component } = await setup();

      component.enqueueModel.set({ topic: 'orders', payload: '{}' });
      TestBed.runInInjectionContext(() => component.addMetadataRow());
      expect(component.metadataRows().length).toBe(1);

      fixture.componentRef.setInput('success', 'some-id');
      fixture.detectChanges();
      await fixture.whenStable();

      expect(component.enqueueModel()).toEqual({ topic: '', payload: '' });
      expect(component.metadataRows().length).toBe(0);
    });
  });

  describe('when the form is submitted', () => {
    it('should emit an enqueue output with the form values', async () => {
      const { fixture, component } = await setup();

      const emitted: EnqueueRequest[] = [];
      component.enqueue.subscribe((req: EnqueueRequest) => emitted.push(req));

      component.enqueueModel.set({ topic: 'orders', payload: '{"x":1}' });
      fixture.detectChanges();
      await fixture.whenStable();

      const el: HTMLElement = fixture.nativeElement;
      const form = el.querySelector('form') as HTMLFormElement;
      form.dispatchEvent(new Event('submit'));
      await fixture.whenStable();

      expect(emitted.length).toBe(1);
      expect(emitted[0].topic).toBe('orders');
      expect(emitted[0].payload).toBe('{"x":1}');
    });
  });

  describe('Avro schema example', () => {
    it('should show "Fill example" button when topic has a schema', async () => {
      const { fixture, component } = await setup();

      component.topicSchema.set(makeSchema());
      fixture.detectChanges();
      await fixture.whenStable();

      const el: HTMLElement = fixture.nativeElement;
      const btn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Fill example',
      );
      expect(btn).not.toBeUndefined();
    });

    it('should hide "Fill example" button when topic has no schema', async () => {
      const { fixture, component } = await setup();

      component.topicSchema.set(null);
      fixture.detectChanges();
      await fixture.whenStable();

      const el: HTMLElement = fixture.nativeElement;
      const btn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Fill example',
      );
      expect(btn).toBeUndefined();
    });

    it('should hide "Fill example" button when topic field is empty', async () => {
      const queueService = makeQueueService({ getTopicSchemaResult: 'error' });
      const { fixture, component } = await setup({ queueService });

      component.enqueueModel.set({ topic: '', payload: '' });
      component.topicSchema.set(null);
      fixture.detectChanges();
      await fixture.whenStable();

      const el: HTMLElement = fixture.nativeElement;
      const btn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Fill example',
      );
      expect(btn).toBeUndefined();
    });

    it('should populate payload textarea with example JSON on button click', async () => {
      const schema = makeSchema({
        schema_json: JSON.stringify({
          type: 'record',
          name: 'Order',
          fields: [
            { name: 'id', type: 'string' },
            { name: 'amount', type: 'double' },
          ],
        }),
      });
      const { fixture, component } = await setup();

      component.topicSchema.set(schema);
      fixture.detectChanges();
      await fixture.whenStable();

      const el: HTMLElement = fixture.nativeElement;
      const btn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Fill example',
      ) as HTMLButtonElement;
      btn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const expected = JSON.stringify({ id: '', amount: 0 }, null, 2);
      expect(component.enqueueModel().payload).toBe(expected);
    });

    it('should reset topicSchema on success', async () => {
      const { fixture, component } = await setup();

      component.topicSchema.set(makeSchema());
      expect(component.topicSchema()).not.toBeNull();

      fixture.componentRef.setInput('success', 'some-id');
      fixture.detectChanges();
      await fixture.whenStable();

      expect(component.topicSchema()).toBeNull();
    });
  });
});
