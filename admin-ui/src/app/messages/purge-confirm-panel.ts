import { Component, input, output, ChangeDetectionStrategy } from '@angular/core';

@Component({
  selector: 'app-purge-confirm-panel',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="px-6 py-3 border-b border-red-200 bg-red-50 space-y-2">
      <p class="text-sm font-medium text-red-800">Purge messages from "{{ topic() }}"</p>
      <div class="flex gap-3 text-sm">
        @for (status of ['pending', 'processing', 'expired']; track status) {
          <label class="flex items-center gap-1 cursor-pointer">
            <input
              type="checkbox"
              [checked]="statuses().includes(status)"
              (change)="statusToggle.emit(status)"
            />
            {{ status }}
          </label>
        }
      </div>
      <div class="flex gap-2">
        <button
          type="button"
          [disabled]="statuses().length === 0"
          (click)="purgeConfirmed.emit({ topic: topic(), statuses: statuses() })"
          class="px-3 py-1.5 text-sm font-medium text-white bg-red-600 rounded-md hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Confirm purge
        </button>
        <button
          type="button"
          (click)="purgeCancelled.emit()"
          class="px-3 py-1.5 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
        >
          Cancel
        </button>
      </div>
    </div>
  `,
})
export class PurgeConfirmPanel {
  readonly topic = input.required<string>();
  readonly statuses = input.required<string[]>();

  readonly purgeConfirmed = output<{ topic: string; statuses: string[] }>();
  readonly purgeCancelled = output<void>();
  readonly statusToggle = output<string>();
}
