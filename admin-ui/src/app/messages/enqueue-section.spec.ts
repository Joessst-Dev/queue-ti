import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { EnqueueSection } from './enqueue-section';
import { EnqueueRequest } from '../services/queue.service';

const setup = async (opts: {
  success?: string;
  error?: string;
  loading?: boolean;
} = {}) => {
  const { success = '', error = '', loading = false } = opts;

  await TestBed.configureTestingModule({
    imports: [EnqueueSection],
    providers: [provideZonelessChangeDetection()],
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
});
