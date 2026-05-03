import { Component, input, output, signal, ChangeDetectionStrategy } from '@angular/core';
import { inputValue } from '../utils/dom';

@Component({
  selector: 'app-nack-overlay',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div
      data-nack-overlay
      class="absolute right-0 top-1/2 -translate-y-1/2 flex items-center gap-1 bg-white border border-gray-200 rounded shadow-md px-2 py-1 z-20"
    >
      <input
        type="text"
        [value]="nackError()"
        (input)="nackError.set(inputValue($event))"
        placeholder="Reason…"
        class="px-1 py-0.5 text-xs border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-red-400 w-28"
      />
      <button
        (click)="onConfirm()"
        title="Confirm nack"
        class="px-1.5 py-0.5 text-xs font-medium bg-red-100 text-red-800 rounded hover:bg-red-200 cursor-pointer whitespace-nowrap"
      >
        ✓
      </button>
      <button
        (click)="nackCancelled.emit()"
        title="Cancel"
        class="px-1.5 py-0.5 text-xs text-gray-500 hover:text-gray-700 cursor-pointer"
      >
        ✗
      </button>
    </div>
  `,
})
export class NackOverlay {
  readonly messageId = input.required<string>();

  readonly nackConfirmed = output<{ id: string; error: string }>();
  readonly nackCancelled = output<void>();

  readonly nackError = signal('');

  protected readonly inputValue = inputValue;

  onConfirm(): void {
    this.nackConfirmed.emit({ id: this.messageId(), error: this.nackError() });
    this.nackError.set('');
  }
}
