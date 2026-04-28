import { Component, inject, signal, ChangeDetectionStrategy, OnInit } from '@angular/core';
import { QueueService, TopicConfig, ReplayResponse } from '../services/queue.service';

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
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Replayable</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Window</th>
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
                    <td class="px-3 py-2">
                      <input
                        type="checkbox"
                        id="new-replayable"
                        [checked]="editForm().replayable"
                        (change)="patchEditForm('replayable', $any($event.target).checked)"
                        class="h-4 w-4 text-indigo-600 border-gray-300 rounded focus:ring-indigo-500 cursor-pointer"
                      />
                      <label for="new-replayable" class="ml-1 text-sm text-gray-700">Replayable</label>
                    </td>
                    <td class="px-3 py-2">
                      @if (editForm().replayable) {
                        <input
                          type="number"
                          [value]="editForm().replay_window_seconds"
                          (input)="patchEditForm('replay_window_seconds', $any($event.target).value)"
                          placeholder="always"
                          aria-label="Window seconds"
                          class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                        />
                      }
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
                      <td class="px-3 py-2">
                        <input
                          type="checkbox"
                          [id]="'replayable-' + cfg.topic"
                          [checked]="editForm().replayable"
                          (change)="patchEditForm('replayable', $any($event.target).checked)"
                          class="h-4 w-4 text-indigo-600 border-gray-300 rounded focus:ring-indigo-500 cursor-pointer"
                        />
                        <label [for]="'replayable-' + cfg.topic" class="ml-1 text-sm text-gray-700">Replayable</label>
                      </td>
                      <td class="px-3 py-2">
                        @if (editForm().replayable) {
                          <input
                            type="number"
                            [value]="editForm().replay_window_seconds"
                            (input)="patchEditForm('replay_window_seconds', $any($event.target).value)"
                            placeholder="always"
                            [attr.aria-label]="'Window seconds for ' + cfg.topic"
                            class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                          />
                        }
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
                      <td class="px-3 py-2">
                        @if (cfg.replayable) {
                          <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800">Yes</span>
                        } @else {
                          <span class="text-gray-400">—</span>
                        }
                      </td>
                      <td class="px-3 py-2">
                        @if (cfg.replayable) {
                          @if (cfg.replay_window_seconds !== null && cfg.replay_window_seconds !== undefined) {
                            {{ cfg.replay_window_seconds }}
                          } @else {
                            Always
                          }
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
                        @if (cfg.replayable) {
                          <button
                            type="button"
                            (click)="onReplay(cfg)"
                            [attr.aria-label]="'Replay ' + cfg.topic"
                            class="px-3 py-1 text-sm font-medium text-indigo-600 border border-indigo-300 rounded-md hover:bg-indigo-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer"
                          >
                            Replay
                          </button>
                        }
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
                    @if (replayingTopic() === cfg.topic) {
                      <tr>
                        <td colspan="7" class="px-4 py-4 bg-indigo-50 border-t border-indigo-100">
                          <div class="space-y-3">
                            <div class="flex flex-col gap-1">
                              <label
                                [for]="'replay-from-' + cfg.topic"
                                class="text-sm font-medium text-gray-700"
                              >
                                {{ cfg.replay_window_seconds !== null && cfg.replay_window_seconds !== undefined ? 'Replay from' : 'Replay from (optional)' }}
                              </label>
                              <input
                                type="datetime-local"
                                [id]="'replay-from-' + cfg.topic"
                                [value]="replayFromTime()"
                                (input)="replayFromTime.set($any($event.target).value)"
                                [min]="replayMinTime()"
                                class="w-64 px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                              />
                              <p class="text-xs text-gray-500">
                                @if (cfg.replay_window_seconds !== null && cfg.replay_window_seconds !== undefined) {
                                  Re-enqueues all messages acked since the selected time. Duplicates may occur if consumers are currently active.
                                } @else {
                                  No time window configured — replaying without a from-time will re-enqueue all archived messages for this topic.
                                }
                              </p>
                            </div>

                            @if (replayResult()) {
                              <div class="p-3 bg-green-50 border border-green-200 text-green-700 rounded text-sm">
                                {{ replayResult()!.enqueued }} messages re-enqueued.
                                @if (replayResult()!.from_time) {
                                  From: {{ replayResult()!.from_time }}
                                }
                              </div>
                            }

                            @if (replayError()) {
                              <div class="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
                                {{ replayError() }}
                              </div>
                            }

                            <div class="flex items-center gap-3">
                              <button
                                type="button"
                                [disabled]="replayLoading()"
                                (click)="onConfirmReplay(cfg.topic)"
                                class="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                @if (replayLoading()) {
                                  <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                                  </svg>
                                }
                                Confirm Replay
                              </button>
                              <button
                                type="button"
                                (click)="onCancelReplay()"
                                class="text-sm text-gray-500 hover:text-gray-700 focus:outline-none cursor-pointer"
                              >
                                Cancel
                              </button>
                            </div>

                            @if (cfg.replay_window_seconds === null || cfg.replay_window_seconds === undefined) {
                              <hr class="border-indigo-200" />
                              <div class="space-y-2">
                                <p class="text-sm font-medium text-gray-700">Trim Archive</p>
                                <p class="text-xs text-gray-500">
                                  Permanently delete archived entries acked before the selected date.
                                  This cannot be undone.
                                </p>
                                <div class="flex flex-col gap-1">
                                  <label
                                    [for]="'trim-before-' + cfg.topic"
                                    class="text-sm text-gray-600"
                                  >Delete entries acked before</label>
                                  <input
                                    type="datetime-local"
                                    [id]="'trim-before-' + cfg.topic"
                                    [value]="trimBeforeTime()"
                                    (input)="trimBeforeTime.set($any($event.target).value)"
                                    class="w-64 px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-red-400"
                                  />
                                </div>

                                @if (trimResult() !== null) {
                                  <div class="p-3 bg-green-50 border border-green-200 text-green-700 rounded text-sm">
                                    {{ trimResult() }} archive entries deleted.
                                  </div>
                                }

                                @if (trimError()) {
                                  <div class="p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
                                    {{ trimError() }}
                                  </div>
                                }

                                <button
                                  type="button"
                                  [disabled]="trimLoading()"
                                  (click)="onConfirmTrim(cfg.topic)"
                                  class="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-red-600 rounded-md hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                                >
                                  @if (trimLoading()) {
                                    <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                                    </svg>
                                  }
                                  Trim Archive
                                </button>
                              </div>
                            }
                          </div>
                        </td>
                      </tr>
                    }
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
  readonly editForm = signal<{
    max_retries: string;
    message_ttl_seconds: string;
    max_depth: string;
    replayable: boolean;
    replay_window_seconds: string;
  }>({
    max_retries: '',
    message_ttl_seconds: '',
    max_depth: '',
    replayable: false,
    replay_window_seconds: '',
  });
  readonly newTopicName = signal('');

  readonly replayingTopic = signal<string | null>(null);
  readonly replayFromTime = signal<string>('');
  readonly replayMinTime = signal<string>('');
  readonly replayLoading = signal(false);
  readonly replayResult = signal<ReplayResponse | null>(null);
  readonly replayError = signal<string | null>(null);

  readonly trimBeforeTime = signal<string>('');
  readonly trimLoading = signal(false);
  readonly trimResult = signal<number | null>(null);
  readonly trimError = signal<string | null>(null);

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
    this.editForm.set({
      max_retries: '',
      message_ttl_seconds: '',
      max_depth: '',
      replayable: false,
      replay_window_seconds: '',
    });
    this.editingTopic.set('__new__');
  }

  onEdit(cfg: TopicConfig): void {
    this.editForm.set({
      max_retries: cfg.max_retries != null ? String(cfg.max_retries) : '',
      message_ttl_seconds: cfg.message_ttl_seconds != null ? String(cfg.message_ttl_seconds) : '',
      max_depth: cfg.max_depth != null ? String(cfg.max_depth) : '',
      replayable: cfg.replayable ?? false,
      replay_window_seconds:
        cfg.replayable && cfg.replay_window_seconds !== null && cfg.replay_window_seconds !== undefined ? String(cfg.replay_window_seconds) : '',
    });
    this.editingTopic.set(cfg.topic);
  }

  onCancel(): void {
    this.editingTopic.set(null);
  }

  patchEditForm(
    field: 'max_retries' | 'message_ttl_seconds' | 'max_depth' | 'replayable' | 'replay_window_seconds',
    value: string | boolean,
  ): void {
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
      replayable: f.replayable,
      replay_window_seconds: f.replayable ? this.parseField(f.replay_window_seconds) : null,
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

  onReplay(cfg: TopicConfig): void {
    this.replayResult.set(null);
    this.replayError.set(null);

    if (cfg.replay_window_seconds !== null && cfg.replay_window_seconds !== undefined) {
      const now = new Date();
      const fromDate = new Date(now.getTime() - cfg.replay_window_seconds * 1000);
      this.replayFromTime.set(this.toDatetimeLocal(fromDate));
      this.replayMinTime.set(this.toDatetimeLocal(fromDate));
    } else {
      this.replayFromTime.set('');
      this.replayMinTime.set('');
    }

    this.replayingTopic.set(cfg.topic);
  }

  onCancelReplay(): void {
    this.replayingTopic.set(null);
    this.replayFromTime.set('');
    this.replayMinTime.set('');
    this.replayResult.set(null);
    this.replayError.set(null);
    this.trimBeforeTime.set('');
    this.trimResult.set(null);
    this.trimError.set(null);
  }

  onConfirmTrim(topic: string): void {
    const beforeTime = this.trimBeforeTime().trim();
    if (!beforeTime) {
      this.trimError.set('Please select a date before trimming');
      return;
    }
    this.trimLoading.set(true);
    this.trimResult.set(null);
    this.trimError.set(null);
    const iso = new Date(beforeTime).toISOString();
    this.queue.trimMessageLog(topic, iso).subscribe({
      next: (res) => {
        this.trimResult.set(res.deleted);
        this.trimLoading.set(false);
      },
      error: (err: { error?: { error?: string } }) => {
        this.trimError.set(err.error?.error ?? 'Failed to trim archive');
        this.trimLoading.set(false);
      },
    });
  }

  onConfirmReplay(topic: string): void {
    this.replayLoading.set(true);
    this.replayResult.set(null);
    this.replayError.set(null);

    const fromTime = this.replayFromTime().trim();
    const isoFromTime = fromTime ? new Date(fromTime).toISOString() : undefined;

    this.queue.replayTopic(topic, isoFromTime).subscribe({
      next: (res) => {
        this.replayResult.set(res);
        this.replayLoading.set(false);
      },
      error: (err: { error?: { error?: string } }) => {
        this.replayError.set(err.error?.error ?? 'Failed to replay topic');
        this.replayLoading.set(false);
      },
    });
  }

  private toDatetimeLocal(date: Date): string {
    const pad = (n: number) => n.toString().padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
  }
}
