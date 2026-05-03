import { Component, inject, input, signal, ChangeDetectionStrategy, OnInit } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { QueueService } from '../../services/queue.service';
import { getErrorMessage } from '../../utils/error';

@Component({
  selector: 'app-consumer-groups-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="bg-white shadow rounded-lg">
      <div class="px-6 py-4 border-b border-gray-200">
        <h2 class="flex items-center gap-2 text-lg font-semibold text-gray-900">
          <svg
            class="w-5 h-5 text-gray-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke-width="1.5"
            stroke="currentColor"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm6 3a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Zm-13.5 0a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Z"
            />
          </svg>
          Consumer Groups
        </h2>
      </div>

      <div class="px-6 py-4">
        @if (error()) {
          <div
            class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm"
            role="alert"
          >
            {{ error() }}
          </div>
        }

        @if (loading()) {
          <p class="text-sm text-gray-500">Loading…</p>
        } @else if (groups().length === 0) {
          <p class="text-sm text-gray-500">No consumer groups registered.</p>
        } @else {
          <div class="overflow-x-auto mb-4">
            <table class="min-w-full divide-y divide-gray-200 text-sm">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Group
                  </th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100">
                @for (group of groups(); track group) {
                  <tr class="hover:bg-gray-50">
                    <td class="px-3 py-2 font-medium text-gray-900">{{ group }}</td>
                    <td class="px-3 py-2">
                      <button
                        type="button"
                        (click)="unregisterGroup(group)"
                        [attr.aria-label]="'Delete consumer group ' + group"
                        class="px-2 py-1 text-lg text-gray-400 hover:text-red-600 focus:outline-none cursor-pointer"
                      >
                        &times;
                      </button>
                    </td>
                  </tr>
                }
              </tbody>
            </table>
          </div>
        }

        <form
          class="flex items-center gap-2 mt-4"
          (submit)="$event.preventDefault(); registerGroup()"
        >
          <input
            type="text"
            [value]="newGroupName()"
            (input)="newGroupName.set(inputValue($event))"
            placeholder="Group name"
            aria-label="New consumer group name"
            class="flex-1 max-w-xs px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
          />
          <button
            type="submit"
            [disabled]="!newGroupName().trim()"
            class="px-3 py-1.5 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Register
          </button>
        </form>
      </div>
    </section>
  `,
})
export class ConsumerGroupsSection implements OnInit {
  private readonly queue = inject(QueueService);

  readonly topic = input.required<string>();

  readonly groups = signal<string[]>([]);
  readonly loading = signal(false);
  readonly newGroupName = signal('');
  readonly error = signal<string | null>(null);

  ngOnInit(): void {
    this.loadGroups();
  }

  inputValue(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }

  loadGroups(): void {
    this.loading.set(true);
    this.error.set(null);
    this.queue.listConsumerGroups(this.topic()).subscribe({
      next: (res) => {
        this.groups.set(res.items ?? []);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to load consumer groups'));
        this.loading.set(false);
      },
    });
  }

  registerGroup(): void {
    const name = this.newGroupName().trim();
    if (!name) {
      return;
    }
    this.error.set(null);
    this.queue.registerConsumerGroup(this.topic(), name).subscribe({
      next: () => {
        this.newGroupName.set('');
        this.loadGroups();
      },
      error: (err: unknown) => {
        if (err instanceof HttpErrorResponse && err.status === 409) {
          this.error.set(`Consumer group "${name}" is already registered.`);
        } else {
          this.error.set(getErrorMessage(err, 'Failed to register consumer group'));
        }
      },
    });
  }

  unregisterGroup(group: string): void {
    this.error.set(null);
    this.queue.unregisterConsumerGroup(this.topic(), group).subscribe({
      next: () => {
        this.loadGroups();
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to unregister consumer group'));
      },
    });
  }
}
