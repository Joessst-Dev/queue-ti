import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { NackOverlay } from './nack-overlay';

const MESSAGE_ID = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee';

const setup = async () => {
  await TestBed.configureTestingModule({
    imports: [NackOverlay],
    providers: [provideZonelessChangeDetection()],
  }).compileComponents();

  const fixture = TestBed.createComponent(NackOverlay);
  fixture.componentRef.setInput('messageId', MESSAGE_ID);
  fixture.detectChanges();
  await fixture.whenStable();

  return { fixture, component: fixture.componentInstance };
};

describe('NackOverlay', () => {
  describe('data-nack-overlay attribute', () => {
    it('should be present on the root div', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;
      expect(el.querySelector('[data-nack-overlay]')).not.toBeNull();
    });
  });

  describe('when the confirm button (✓) is clicked with an empty reason', () => {
    it('should emit nackConfirmed with the messageId and an empty error string', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      const emitted: { id: string; error: string }[] = [];
      fixture.componentInstance.nackConfirmed.subscribe((v: { id: string; error: string }) =>
        emitted.push(v),
      );

      const confirmBtn = el.querySelector<HTMLButtonElement>('button[title="Confirm nack"]');
      confirmBtn?.click();
      await fixture.whenStable();

      expect(emitted.length).toBe(1);
      expect(emitted[0]).toEqual({ id: MESSAGE_ID, error: '' });
    });
  });

  describe('when the confirm button is clicked after typing a reason', () => {
    it('should emit nackConfirmed with the typed error string', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      const emitted: { id: string; error: string }[] = [];
      fixture.componentInstance.nackConfirmed.subscribe((v: { id: string; error: string }) =>
        emitted.push(v),
      );

      const input = el.querySelector<HTMLInputElement>('input[type="text"]');
      expect(input).not.toBeNull();
      (input as HTMLInputElement).value = 'downstream failure';
      (input as HTMLInputElement).dispatchEvent(new Event('input'));
      fixture.detectChanges();
      await fixture.whenStable();

      const confirmBtn = el.querySelector<HTMLButtonElement>('button[title="Confirm nack"]');
      confirmBtn?.click();
      await fixture.whenStable();

      expect(emitted.length).toBe(1);
      expect(emitted[0]).toEqual({ id: MESSAGE_ID, error: 'downstream failure' });
    });

    it('should reset the input to empty after confirming', async () => {
      const { fixture, component } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      const input = el.querySelector<HTMLInputElement>('input[type="text"]');
      expect(input).not.toBeNull();
      (input as HTMLInputElement).value = 'downstream failure';
      (input as HTMLInputElement).dispatchEvent(new Event('input'));
      fixture.detectChanges();
      await fixture.whenStable();

      const confirmBtn = el.querySelector<HTMLButtonElement>('button[title="Confirm nack"]');
      confirmBtn?.click();
      await fixture.whenStable();

      expect(component.nackError()).toBe('');
    });
  });

  describe('when the cancel button (✗) is clicked', () => {
    it('should emit nackCancelled', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      let cancelledCount = 0;
      fixture.componentInstance.nackCancelled.subscribe(() => { cancelledCount++; });

      const cancelBtn = el.querySelector<HTMLButtonElement>('button[title="Cancel"]');
      cancelBtn?.click();
      await fixture.whenStable();

      expect(cancelledCount).toBe(1);
    });

    it('should NOT emit nackConfirmed', async () => {
      const { fixture } = await setup();
      const el: HTMLElement = fixture.nativeElement;

      const confirmedEmissions: unknown[] = [];
      fixture.componentInstance.nackConfirmed.subscribe((v: unknown) =>
        confirmedEmissions.push(v),
      );

      const cancelBtn = el.querySelector<HTMLButtonElement>('button[title="Cancel"]');
      cancelBtn?.click();
      await fixture.whenStable();

      expect(confirmedEmissions.length).toBe(0);
    });
  });
});
