import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { QueueService } from '../services/queue.service';

@Component({
  selector: 'app-maintenance-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule],
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
        <div class="px-6 py-4 border-b border-gray-200 flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900">Delete Reaper</h2>
          @if (scheduleActive()) {
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
              Active
            </span>
          } @else {
            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-600">
              Not configured
            </span>
          }
        </div>
        <div class="px-6 py-4 space-y-4">
          <p class="text-sm text-gray-600">
            Permanently deletes all 'expired' messages on the configured schedule.
          </p>

          <!-- Schedule editor -->
          <div class="space-y-2">
            <label class="block text-sm font-medium text-gray-700" for="reaperSchedule">
              Cron schedule
            </label>
            <div class="flex gap-2">
              <input
                id="reaperSchedule"
                type="text"
                [(ngModel)]="scheduleInput"
                placeholder="e.g. 0 2 * * *  (leave blank to disable)"
                class="flex-1 min-w-0 block px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500"
              />
              <button
                type="button"
                [disabled]="scheduleSaving()"
                (click)="onSaveSchedule()"
                class="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
              >
                @if (scheduleSaving()) {
                  <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                  </svg>
                }
                Save
              </button>
            </div>
            <p class="text-xs text-gray-500">Standard 5-field cron (minute hour day month weekday). Leave blank to disable.</p>
          </div>

          @if (scheduleError()) {
            <div class="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
              {{ scheduleError() }}
            </div>
          }

          @if (scheduleSaveSuccess()) {
            <div class="p-3 bg-green-50 border border-green-200 text-green-700 rounded text-sm">
              Schedule saved.
            </div>
          }

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
export class MaintenanceSection implements OnInit {
  private readonly queue = inject(QueueService);

  readonly expiryLoading = signal(false);
  readonly expiryResult = signal<number | null>(null);
  readonly expiryError = signal<string | null>(null);

  readonly deleteLoading = signal(false);
  readonly deleteResult = signal<number | null>(null);
  readonly deleteError = signal<string | null>(null);

  readonly scheduleActive = signal(false);
  readonly scheduleSaving = signal(false);
  readonly scheduleError = signal<string | null>(null);
  readonly scheduleSaveSuccess = signal(false);
  scheduleInput = '';

  ngOnInit(): void {
    this.queue.getDeleteReaperSchedule().subscribe({
      next: (res) => {
        this.scheduleInput = res.schedule;
        this.scheduleActive.set(res.active);
      },
    });
  }

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

  onSaveSchedule(): void {
    this.scheduleSaving.set(true);
    this.scheduleError.set(null);
    this.scheduleSaveSuccess.set(false);
    this.queue.updateDeleteReaperSchedule(this.scheduleInput.trim()).subscribe({
      next: (res) => {
        this.scheduleActive.set(res.active);
        this.scheduleInput = res.schedule;
        this.scheduleSaveSuccess.set(true);
        this.scheduleSaving.set(false);
      },
      error: (err: { error?: { error?: string } }) => {
        this.scheduleError.set(err.error?.error ?? 'Failed to save schedule');
        this.scheduleSaving.set(false);
      },
    });
  }
}
