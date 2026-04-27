import { Component, inject, signal, ChangeDetectionStrategy, OnInit } from '@angular/core';
import { QueueService, TopicConfig } from '../services/queue.service';

@Component({
  selector: 'app-topic-config-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [],
  template: `
    <section class="bg-white shadow rounded-lg">
      <div class="px-6 py-4 border-b border-gray-200">
        <div class="flex items-center justify-between">
          <h2 class="flex items-center gap-2 text-lg font-semibold text-gray-900">
            <svg
              class="w-5 h-5 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke-width="1.5"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z"
              />
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z"
              />
            </svg>
            Topic Configuration
          </h2>
          <button
            type="button"
            (click)="onAddConfig()"
            [disabled]="editingTopic() === '__new__'"
            class="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-indigo-600 border border-indigo-300 rounded-md hover:bg-indigo-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
            </svg>
            New Topic
          </button>
        </div>
      </div>

      <div class="px-6 py-4">
        @if (error()) {
          <div class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
            {{ error() }}
          </div>
        }

        @if (loading()) {
          <p class="text-sm text-gray-500">Loading…</p>
        } @else if (configs().length === 0 && editingTopic() !== '__new__') {
          <p class="text-sm text-gray-500">
            No topics registered. Use 'New Topic' to register a topic before messages can be enqueued to it.
          </p>
        } @else {
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Topic</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Max Retries</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">TTL (seconds)</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Max Depth</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100">
                @if (editingTopic() === '__new__') {
                  <tr class="bg-indigo-50">
                    <td class="px-3 py-2">
                      <input
                        type="text"
                        [value]="newTopicName()"
                        (input)="newTopicName.set($any($event.target).value)"
                        placeholder="topic name"
                        aria-label="New topic name"
                        class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      />
                    </td>
                    <td class="px-3 py-2">
                      <input
                        type="number"
                        [value]="editForm().max_retries"
                        (input)="patchEditForm('max_retries', $any($event.target).value)"
                        placeholder="default"
                        aria-label="Max retries"
                        class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      />
                    </td>
                    <td class="px-3 py-2">
                      <input
                        type="number"
                        [value]="editForm().message_ttl_seconds"
                        (input)="patchEditForm('message_ttl_seconds', $any($event.target).value)"
                        placeholder="default"
                        aria-label="TTL seconds"
                        class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      />
                    </td>
                    <td class="px-3 py-2">
                      <input
                        type="number"
                        [value]="editForm().max_depth"
                        (input)="patchEditForm('max_depth', $any($event.target).value)"
                        placeholder="default"
                        aria-label="Max depth"
                        class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      />
                    </td>
                    <td class="px-3 py-2 flex items-center gap-2">
                      <button
                        type="button"
                        (click)="onSaveNew()"
                        class="px-3 py-1 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer"
                      >
                        Save
                      </button>
                      <button
                        type="button"
                        (click)="onCancel()"
                        class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                      >
                        Cancel
                      </button>
                    </td>
                  </tr>
                }
                @for (cfg of configs(); track cfg.topic) {
                  @if (editingTopic() === cfg.topic) {
                    <tr class="bg-indigo-50">
                      <td class="px-3 py-2 font-medium text-gray-900">{{ cfg.topic }}</td>
                      <td class="px-3 py-2">
                        <input
                          type="number"
                          [value]="editForm().max_retries"
                          (input)="patchEditForm('max_retries', $any($event.target).value)"
                          placeholder="default"
                          [attr.aria-label]="'Max retries for ' + cfg.topic"
                          class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                        />
                      </td>
                      <td class="px-3 py-2">
                        <input
                          type="number"
                          [value]="editForm().message_ttl_seconds"
                          (input)="patchEditForm('message_ttl_seconds', $any($event.target).value)"
                          placeholder="default"
                          [attr.aria-label]="'TTL seconds for ' + cfg.topic"
                          class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                        />
                      </td>
                      <td class="px-3 py-2">
                        <input
                          type="number"
                          [value]="editForm().max_depth"
                          (input)="patchEditForm('max_depth', $any($event.target).value)"
                          placeholder="default"
                          [attr.aria-label]="'Max depth for ' + cfg.topic"
                          class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                        />
                      </td>
                      <td class="px-3 py-2 flex items-center gap-2">
                        <button
                          type="button"
                          (click)="onSave(cfg.topic)"
                          class="px-3 py-1 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer"
                        >
                          Save
                        </button>
                        <button
                          type="button"
                          (click)="onCancel()"
                          class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                        >
                          Cancel
                        </button>
                      </td>
                    </tr>
                  } @else {
                    <tr class="hover:bg-gray-50">
                      <td class="px-3 py-2 font-medium text-gray-900">{{ cfg.topic }}</td>
                      <td class="px-3 py-2">
                        @if (cfg.max_retries !== null && cfg.max_retries !== undefined) {
                          {{ cfg.max_retries }}
                        } @else {
                          <span class="text-gray-400">—</span>
                        }
                      </td>
                      <td class="px-3 py-2">
                        @if (cfg.message_ttl_seconds !== null && cfg.message_ttl_seconds !== undefined) {
                          {{ cfg.message_ttl_seconds }}
                        } @else {
                          <span class="text-gray-400">—</span>
                        }
                      </td>
                      <td class="px-3 py-2">
                        @if (cfg.max_depth !== null && cfg.max_depth !== undefined) {
                          {{ cfg.max_depth }}
                        } @else {
                          <span class="text-gray-400">—</span>
                        }
                      </td>
                      <td class="px-3 py-2 flex items-center gap-2">
                        <button
                          type="button"
                          (click)="onEdit(cfg)"
                          [attr.aria-label]="'Edit ' + cfg.topic"
                          class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          (click)="onDelete(cfg.topic)"
                          [attr.aria-label]="'Delete ' + cfg.topic"
                          class="px-2 py-1 text-lg text-gray-400 hover:text-red-600 focus:outline-none cursor-pointer"
                        >
                          &times;
                        </button>
                      </td>
                    </tr>
                  }
                }
              </tbody>
            </table>
          </div>
        }
      </div>
    </section>
  `,
})
export class TopicConfigSection implements OnInit {
  private readonly queue = inject(QueueService);

  readonly configs = signal<TopicConfig[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');
  readonly editingTopic = signal<string | null>(null);
  readonly editForm = signal<{ max_retries: string; message_ttl_seconds: string; max_depth: string }>({
    max_retries: '',
    message_ttl_seconds: '',
    max_depth: '',
  });
  readonly newTopicName = signal('');

  ngOnInit(): void {
    this.loadConfigs();
  }

  private loadConfigs(): void {
    this.loading.set(true);
    this.error.set('');
    this.queue.getTopicConfigs().subscribe({
      next: (res) => {
        this.configs.set(res.items);
        this.loading.set(false);
      },
      error: (err: { error?: { error?: string } }) => {
        this.error.set(err.error?.error ?? 'Failed to load topic configs');
        this.loading.set(false);
      },
    });
  }

  onAddConfig(): void {
    this.newTopicName.set('');
    this.editForm.set({ max_retries: '', message_ttl_seconds: '', max_depth: '' });
    this.editingTopic.set('__new__');
  }

  onEdit(cfg: TopicConfig): void {
    this.editForm.set({
      max_retries: cfg.max_retries != null ? String(cfg.max_retries) : '',
      message_ttl_seconds: cfg.message_ttl_seconds != null ? String(cfg.message_ttl_seconds) : '',
      max_depth: cfg.max_depth != null ? String(cfg.max_depth) : '',
    });
    this.editingTopic.set(cfg.topic);
  }

  onCancel(): void {
    this.editingTopic.set(null);
  }

  patchEditForm(field: 'max_retries' | 'message_ttl_seconds' | 'max_depth', value: string): void {
    this.editForm.update((f) => ({ ...f, [field]: value }));
  }

  private parseField(value: string): number | null {
    const trimmed = value.trim();
    if (trimmed === '') return null;
    const parsed = parseInt(trimmed, 10);
    return isNaN(parsed) ? null : parsed;
  }

  private buildCfg(): Omit<TopicConfig, 'topic'> {
    const f = this.editForm();
    return {
      max_retries: this.parseField(f.max_retries),
      message_ttl_seconds: this.parseField(f.message_ttl_seconds),
      max_depth: this.parseField(f.max_depth),
    };
  }

  onSave(topic: string): void {
    const cfg = this.buildCfg();
    this.queue.upsertTopicConfig(topic, cfg).subscribe({
      next: (updated) => {
        this.configs.update((list) =>
          list.map((c) => (c.topic === topic ? updated : c)),
        );
        this.editingTopic.set(null);
      },
      error: (err: { error?: { error?: string } }) => {
        this.error.set(err.error?.error ?? 'Failed to save topic config');
      },
    });
  }

  onSaveNew(): void {
    const topic = this.newTopicName().trim();
    if (!topic) {
      this.error.set('Topic name is required');
      return;
    }
    const cfg = this.buildCfg();
    this.queue.upsertTopicConfig(topic, cfg).subscribe({
      next: (created) => {
        this.configs.update((list) => [...list, created]);
        this.editingTopic.set(null);
        this.newTopicName.set('');
      },
      error: (err: { error?: { error?: string } }) => {
        this.error.set(err.error?.error ?? 'Failed to save topic config');
      },
    });
  }

  onDelete(topic: string): void {
    this.queue.deleteTopicConfig(topic).subscribe({
      next: () => {
        this.configs.update((list) => list.filter((c) => c.topic !== topic));
      },
      error: (err: { error?: { error?: string } }) => {
        this.error.set(err.error?.error ?? 'Failed to delete topic config');
      },
    });
  }
}
