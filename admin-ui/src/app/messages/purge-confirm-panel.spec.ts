import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { PurgeConfirmPanel } from './purge-confirm-panel';

const DEFAULT_TOPIC = 'orders';
const DEFAULT_STATUSES = ['pending', 'processing', 'expired'];

const setup = async (opts: { topic?: string; statuses?: string[] } = {}) => {
  const { topic = DEFAULT_TOPIC, statuses = DEFAULT_STATUSES } = opts;

  await TestBed.configureTestingModule({
    imports: [PurgeConfirmPanel],
    providers: [provideZonelessChangeDetection()],
  }).compileComponents();

  const fixture = TestBed.createComponent(PurgeConfirmPanel);
  fixture.componentRef.setInput('topic', topic);
  fixture.componentRef.setInput('statuses', statuses);
  fixture.detectChanges();
  await fixture.whenStable();

  return { fixture, component: fixture.componentInstance };
};

describe('PurgeConfirmPanel', () => {
  describe('when rendered with a topic', () => {
    it('should render the topic name in the panel heading', async () => {
      const { fixture } = await setup({ topic: 'orders' });
      const el: HTMLElement = fixture.nativeElement;
      expect(el.textContent).toContain('Purge messages from "orders"');
    });
  });

  describe('when Confirm purge is clicked', () => {
    it('should emit purgeConfirmed with the topic and statuses', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      const emitted: { topic: string; statuses: string[] }[] = [];
      fixture.componentInstance.purgeConfirmed.subscribe(
        (v: { topic: string; statuses: string[] }) => emitted.push(v),
      );

      const confirmBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Confirm purge',
      ) as HTMLButtonElement;
      confirmBtn.click();
      await fixture.whenStable();

      expect(emitted.length).toBe(1);
      expect(emitted[0]).toEqual({ topic: DEFAULT_TOPIC, statuses: DEFAULT_STATUSES });
    });
  });

  describe('when Cancel is clicked', () => {
    it('should emit purgeCancelled', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      let cancelledCount = 0;
      fixture.componentInstance.purgeCancelled.subscribe(() => { cancelledCount++; });

      const cancelBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Cancel',
      ) as HTMLButtonElement;
      cancelBtn.click();
      await fixture.whenStable();

      expect(cancelledCount).toBe(1);
    });
  });

  describe('when a status checkbox is toggled', () => {
    it('should emit statusToggle with the status string', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      const emitted: string[] = [];
      fixture.componentInstance.statusToggle.subscribe((v: string) => emitted.push(v));

      const checkboxes = el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]');
      // Toggle the first checkbox (pending)
      checkboxes[0].dispatchEvent(new Event('change'));
      await fixture.whenStable();

      expect(emitted.length).toBe(1);
      expect(emitted[0]).toBe('pending');
    });

    it('should emit statusToggle with the correct status for each checkbox', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      const emitted: string[] = [];
      fixture.componentInstance.statusToggle.subscribe((v: string) => emitted.push(v));

      const checkboxes = el.querySelectorAll<HTMLInputElement>('input[type="checkbox"]');
      checkboxes[1].dispatchEvent(new Event('change'));
      await fixture.whenStable();

      expect(emitted[0]).toBe('processing');
    });
  });

  describe('when statuses input is empty', () => {
    it('should disable the Confirm purge button', async () => {
      const { fixture } = await setup({ statuses: [] });
      const el: HTMLElement = fixture.nativeElement;

      const confirmBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Confirm purge',
      ) as HTMLButtonElement;

      expect(confirmBtn.disabled).toBe(true);
    });
  });
});
