import { Component, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { QueueService } from '../services/queue.service';

@Component({
  selector: 'app-maintenance-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [],
  template: `
    <section class="space-y-6">
      <!-- Expiry Reaper -->
      <div class="bg-white shadow rounded-lg">
        <div class="px-6 py-4 border-b border-gray-200">
          <h2 class="text-lg font-semibold text-gray-900">Expiry Reaper</h2>
        </div>
        <div class="px-6 py-4 space-y-3">
          <p class="text-sm text-gray-600">
            Marks messages with a passed expiry time as 'expired'. Runs automatically every 60 seconds.
          </p>

          @if (expiryError()) {
            <div class="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
              {{ expiryError() }}
            </div>
          }

          @if (expiryResult() !== null) {
            <div class="p-3 bg-green-50 border border-green-200 text-green-700 rounded text-sm">
              {{ expiryResult() }} messages marked as expired.
            </div>
          }

          <button
            type="button"
            [disabled]="expiryLoading()"
            (click)="onRunExpiryReaper()"
            class="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            @if (expiryLoading()) {
              <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
              </svg>
            }
            Run Now
          </button>
        </div>
      </div>

      <!-- Delete Reaper -->
      <div class="bg-white shadow rounded-lg">
        <div class="px-6 py-4 border-b border-gray-200">
          <h2 class="text-lg font-semibold text-gray-900">Delete Reaper</h2>
        </div>
        <div class="px-6 py-4 space-y-3">
          <p class="text-sm text-gray-600">
            Permanently deletes all 'expired' messages. Configure a schedule server-side via
            <code class="px-1 py-0.5 bg-gray-100 rounded text-xs font-mono">QUEUETI_QUEUE_DELETE_REAPER_SCHEDULE</code>
            (e.g. "0 2 * * *").
          </p>

          @if (deleteError()) {
            <div class="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
              {{ deleteError() }}
            </div>
          }

          @if (deleteResult() !== null) {
            <div class="p-3 bg-green-50 border border-green-200 text-green-700 rounded text-sm">
              {{ deleteResult() }} expired messages deleted.
            </div>
          }

          <button
            type="button"
            [disabled]="deleteLoading()"
            (click)="onRunDeleteReaper()"
            class="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            @if (deleteLoading()) {
              <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
              </svg>
            }
            Run Now
          </button>
        </div>
      </div>
    </section>
  `,
})
export class MaintenanceSection {
  private readonly queue = inject(QueueService);

  readonly expiryLoading = signal(false);
  readonly expiryResult = signal<number | null>(null);
  readonly expiryError = signal<string | null>(null);

  readonly deleteLoading = signal(false);
  readonly deleteResult = signal<number | null>(null);
  readonly deleteError = signal<string | null>(null);

  onRunExpiryReaper(): void {
    this.expiryLoading.set(true);
    this.expiryResult.set(null);
    this.expiryError.set(null);
    this.queue.runExpiryReaper().subscribe({
      next: (res) => {
        this.expiryResult.set(res.expired);
        this.expiryLoading.set(false);
      },
      error: (err: { error?: { error?: string } }) => {
        this.expiryError.set(err.error?.error ?? 'Failed to run expiry reaper');
        this.expiryLoading.set(false);
      },
    });
  }

  onRunDeleteReaper(): void {
    this.deleteLoading.set(true);
    this.deleteResult.set(null);
    this.deleteError.set(null);
    this.queue.runDeleteReaper().subscribe({
      next: (res) => {
        this.deleteResult.set(res.deleted);
        this.deleteLoading.set(false);
      },
      error: (err: { error?: { error?: string } }) => {
        this.deleteError.set(err.error?.error ?? 'Failed to run delete reaper');
        this.deleteLoading.set(false);
      },
    });
  }
}
