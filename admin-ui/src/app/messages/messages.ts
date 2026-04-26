import { Component, inject, computed, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { FormField, form, schema, required } from '@angular/forms/signals';
import { SlicePipe, DatePipe } from '@angular/common';
import { Router } from '@angular/router';
import { Subject, switchMap, map, catchError, of, startWith, tap } from 'rxjs';
import { CdkVirtualScrollViewport, CdkVirtualForOf, CdkFixedSizeVirtualScroll } from '@angular/cdk/scrolling';
import { AuthService } from '../services/auth.service';
import {
  QueueService,
  QueueMessage,
  EnqueueRequest,
  PAGE_SIZE,
} from '../services/queue.service';

interface EnqueueState {
  id: string;
  loading: boolean;
  error: string;
}

interface EnqueueModel {
  topic: string;
  payload: string;
}

interface MetadataRowModel {
  key: string;
  value: string;
}

@Component({
  selector: 'app-messages',
  imports: [FormField, SlicePipe, DatePipe, CdkVirtualScrollViewport, CdkVirtualForOf, CdkFixedSizeVirtualScroll],
  template: `
    <div class="min-h-screen bg-gray-50">
      <!-- Header -->
      <header class="bg-white shadow-sm">
        <div
          class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4 flex items-center justify-between"
        >
          <div class="flex items-center gap-2">
            <svg
              class="w-6 h-6 text-indigo-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke-width="1.5"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M3.75 12h16.5m-16.5 3.75h16.5M3.75 19.5h16.5M5.625 4.5h12.75a1.875 1.875 0 0 1 0 3.75H5.625a1.875 1.875 0 0 1 0-3.75Z"
              />
            </svg>
            <h1 class="text-xl font-bold text-gray-900">QueueTI Admin</h1>
          </div>
          @if (auth.isAuthenticated()) {
            <button
              (click)="onLogout()"
              class="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 cursor-pointer"
            >
              <svg
                class="w-4 h-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke-width="1.5"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M8.25 9V5.25A2.25 2.25 0 0 1 10.5 3h6a2.25 2.25 0 0 1 2.25 2.25v13.5A2.25 2.25 0 0 1 16.5 21h-6a2.25 2.25 0 0 1-2.25-2.25V15m-3 0-3-3m0 0 3-3m-3 3H15"
                />
              </svg>
              Sign out
            </button>
          }
        </div>
      </header>

      <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-8">
        <!-- Messages Section -->
        <section class="bg-white shadow rounded-lg">
          <div
            class="px-6 py-4 border-b border-gray-200 flex items-center justify-between flex-wrap gap-4"
          >
            <h2
              class="flex items-center gap-2 text-lg font-semibold text-gray-900"
            >
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
                  d="M8.25 6.75h12M8.25 12h12m-12 5.25h12M3.75 6.75h.007v.008H3.75V6.75Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0ZM3.75 12h.007v.008H3.75V12Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Zm-.375 5.25h.007v.008H3.75v-.008Zm.375 0a.375.375 0 1 1-.75 0 .375.375 0 0 1 .75 0Z"
                />
              </svg>
              Messages
            </h2>
            <div class="flex items-center gap-3">
              <input
                type="text"
                placeholder="Filter by topic..."
                [formField]="filterForm"
                (keydown.enter)="loadMessages()"
                class="px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
              <button
                (click)="loadMessages()"
                class="px-3 py-1.5 text-sm bg-gray-100 text-gray-700 rounded-md hover:bg-gray-200 cursor-pointer"
              >
                <svg
                  class="inline w-4 h-4 mr-1"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182"
                  />
                </svg>
                Refresh
              </button>
            </div>
          </div>

          @if (messagesError()) {
            <div class="px-6 py-4 text-sm text-red-600">
              {{ messagesError() }}
            </div>
          }

          <div class="overflow-x-auto">
            <cdk-virtual-scroll-viewport [itemSize]="73" style="height: 520px;">
              <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50 sticky top-0 z-10">
                  <tr>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      ID
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Topic
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Payload
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Status
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Retries
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Expires
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Metadata
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Created
                    </th>
                    <th
                      class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase"
                    >
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-200">
                  <tr *cdkVirtualFor="let msg of allMessages(); trackBy: trackByMsgId" [class]="rowClasses(msg)">
                    <td class="px-6 py-4 text-sm font-mono text-gray-600">
                      <div class="flex items-center gap-1">
                        <span [title]="msg.id"
                          >{{ msg.id | slice: 0 : 8 }}&hellip;</span
                        >
                        <button
                          type="button"
                          (click)="copyId(msg.id)"
                          [title]="'Copy full ID: ' + msg.id"
                          class="text-gray-400 hover:text-gray-600 cursor-pointer"
                        >
                          @if (copiedId() === msg.id) {
                            <svg class="w-3.5 h-3.5 text-green-500" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor">
                              <path stroke-linecap="round" stroke-linejoin="round" d="m4.5 12.75 6 6 9-13.5" />
                            </svg>
                          } @else {
                            <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
                              <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 17.25v3.375c0 .621-.504 1.125-1.125 1.125h-9.75a1.125 1.125 0 0 1-1.125-1.125V7.875c0-.621.504-1.125 1.125-1.125H6.75a9.06 9.06 0 0 1 1.5.124m7.5 10.376h3.375c.621 0 1.125-.504 1.125-1.125V11.25c0-4.46-3.243-8.161-7.5-8.876a9.06 9.06 0 0 0-1.5-.124H9.375c-.621 0-1.125.504-1.125 1.125v3.5m7.5 10.375H9.375a1.125 1.125 0 0 1-1.125-1.125v-9.25m12 6.625v-1.875a3.375 3.375 0 0 0-3.375-3.375h-1.5a1.125 1.125 0 0 1-1.125-1.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H9.75" />
                            </svg>
                          }
                        </button>
                      </div>
                    </td>
                    <td class="px-6 py-4 text-sm text-gray-900">
                      <div>{{ msg.topic }}</div>
                      @if (msg.original_topic) {
                        <div class="text-xs text-gray-400 mt-0.5">
                          from: {{ msg.original_topic }}
                        </div>
                      }
                    </td>
                    <td
                      class="px-6 py-4 text-sm text-gray-600 max-w-xs truncate font-mono"
                    >
                      {{ msg.payload }}
                    </td>
                    <td class="px-6 py-4 text-sm">
                      <span
                        class="inline-flex px-2 py-0.5 text-xs font-medium rounded-full"
                        [class]="statusClasses(msg.status)"
                      >
                        {{ msg.status }}
                      </span>
                    </td>
                    <td class="px-6 py-4 text-sm">
                      <span
                        [title]="msg.last_error || ''"
                        [class]="retriesExhausted(msg) ? 'text-red-600 font-medium' : 'text-gray-500'"
                      >
                        {{ msg.retry_count }} / {{ msg.max_retries }}
                      </span>
                    </td>
                    <td class="px-6 py-4 text-sm text-gray-500 whitespace-nowrap">
                      @if (msg.expires_at) {
                        {{ msg.expires_at | date: 'short' }}
                      } @else {
                        <span class="text-gray-400">&mdash;</span>
                      }
                    </td>
                    <td class="px-6 py-4 text-sm text-gray-500">
                      @if (
                        msg.metadata && objectKeys(msg.metadata).length > 0
                      ) {
                        @for (key of objectKeys(msg.metadata); track key) {
                          <span
                            class="inline-flex items-center px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-700 mr-1 mb-1"
                          >
                            {{ key }}={{ msg.metadata[key] }}
                          </span>
                        }
                      } @else {
                        <span class="text-gray-400">&mdash;</span>
                      }
                    </td>
                    <td
                      class="px-6 py-4 text-sm text-gray-500 whitespace-nowrap"
                    >
                      {{ msg.created_at | date: 'short' }}
                    </td>
                    <td class="px-6 py-4 text-sm whitespace-nowrap">
                      @if (isDlq(msg)) {
                        <button
                          (click)="onRequeue(msg.id)"
                          class="px-3 py-1 text-xs font-medium bg-amber-100 text-amber-800 rounded hover:bg-amber-200 cursor-pointer"
                        >
                          Requeue
                        </button>
                      } @else if (msg.status === 'processing') {
                        @if (nackOpenId() === msg.id) {
                          <div class="flex items-center gap-1">
                            <input
                              type="text"
                              [value]="nackError()"
                              (input)="nackError.set($any($event.target).value)"
                              placeholder="Error reason (optional)"
                              class="px-2 py-1 text-xs border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-red-400 w-40"
                            />
                            <button
                              (click)="onNackConfirm(msg.id)"
                              class="px-2 py-1 text-xs font-medium bg-red-100 text-red-800 rounded hover:bg-red-200 cursor-pointer"
                            >
                              Confirm
                            </button>
                            <button
                              (click)="nackOpenId.set(null)"
                              class="px-2 py-1 text-xs text-gray-500 hover:text-gray-700 cursor-pointer"
                            >
                              Cancel
                            </button>
                          </div>
                        } @else {
                          <button
                            (click)="nackOpenId.set(msg.id); nackError.set('')"
                            class="px-3 py-1 text-xs font-medium bg-red-100 text-red-800 rounded hover:bg-red-200 cursor-pointer"
                          >
                            Nack
                          </button>
                        }
                      }
                    </td>
                  </tr>
                  @if (allMessages().length === 0 && !loadingMessages()) {
                    <tr>
                      <td
                        colspan="9"
                        class="px-6 py-12 text-center text-sm text-gray-500"
                      >
                        No messages found
                      </td>
                    </tr>
                  }
                  @if (allMessages().length === 0 && loadingMessages()) {
                    <tr>
                      <td
                        colspan="9"
                        class="px-6 py-12 text-center text-sm text-gray-500"
                      >
                        Loading messages...
                      </td>
                    </tr>
                  }
                </tbody>
              </table>
            </cdk-virtual-scroll-viewport>
          </div>
        </section>

        <!-- Enqueue Section -->
        <section class="bg-white shadow rounded-lg">
          <div class="px-6 py-4 border-b border-gray-200">
            <h2
              class="flex items-center gap-2 text-lg font-semibold text-gray-900"
            >
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
                  d="M12 9v6m3-3H9m12 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
                />
              </svg>
              Enqueue Message
            </h2>
          </div>
          <div class="px-6 py-4">
            @if (enqueueSuccess()) {
              <div
                class="mb-4 p-3 bg-green-50 border border-green-200 text-green-700 rounded text-sm"
              >
                Message enqueued successfully (ID: {{ enqueueSuccess() }})
              </div>
            }
            @if (enqueueError()) {
              <div
                class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm"
              >
                {{ enqueueError() }}
              </div>
            }

            <form (submit)="onEnqueue($event)" class="space-y-4">
              <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label
                    for="enqueue-topic"
                    class="block text-sm font-medium text-gray-700 mb-1"
                    >Topic</label
                  >
                  <input
                    id="enqueue-topic"
                    type="text"
                    [formField]="enqueueForm.topic"
                    class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    placeholder="e.g. orders"
                  />
                </div>
              </div>

              <div>
                <label
                  for="enqueue-payload"
                  class="block text-sm font-medium text-gray-700 mb-1"
                >
                  Payload
                </label>
                <textarea
                  id="enqueue-payload"
                  [formField]="enqueueForm.payload"
                  rows="4"
                  class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 font-mono text-sm"
                  placeholder='{"key": "value"}'
                ></textarea>
              </div>

              <!-- Metadata -->
              <div>
                <div class="flex items-center justify-between mb-2">
                  <label
                    for="add-metadata-field"
                    class="block text-sm font-medium text-gray-700"
                  >
                    Metadata
                  </label>
                  <button
                    id="add-metadata-field"
                    type="button"
                    (click)="addMetadataRow()"
                    class="text-sm text-indigo-600 hover:text-indigo-800 cursor-pointer"
                  >
                    <svg
                      class="inline w-4 h-4 mr-0.5"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke-width="1.5"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        d="M12 4.5v15m7.5-7.5h-15"
                      />
                    </svg>
                    Add field
                  </button>
                </div>
                @for (row of metadataRows(); track $index) {
                  <div class="flex gap-2 mb-2">
                    <input
                      type="text"
                      [formField]="row.form.key"
                      [attr.aria-label]="'Metadata key ' + ($index + 1)"
                      placeholder="Key"
                      class="flex-1 px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    />
                    <input
                      type="text"
                      [formField]="row.form.value"
                      [attr.aria-label]="'Metadata value ' + ($index + 1)"
                      placeholder="Value"
                      class="flex-1 px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    />
                    <button
                      type="button"
                      (click)="removeMetadataRow($index)"
                      class="px-2 text-gray-400 hover:text-red-600 text-lg cursor-pointer"
                    >
                      &times;
                    </button>
                  </div>
                }
              </div>

              <button
                type="submit"
                [disabled]="enqueueLoading()"
                class="px-4 py-2 bg-indigo-600 text-white font-medium rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                @if (enqueueLoading()) {
                  <svg
                    class="inline w-4 h-4 mr-1 animate-spin"
                    fill="none"
                    viewBox="0 0 24 24"
                  >
                    <circle
                      class="opacity-25"
                      cx="12"
                      cy="12"
                      r="10"
                      stroke="currentColor"
                      stroke-width="4"
                    ></circle>
                    <path
                      class="opacity-75"
                      fill="currentColor"
                      d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                    ></path>
                  </svg>
                  Sending...
                } @else {
                  <svg
                    class="inline w-4 h-4 mr-1"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke-width="1.5"
                    stroke="currentColor"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M6 12 3.269 3.125A59.769 59.769 0 0 1 21.485 12 59.768 59.768 0 0 1 3.27 20.875L5.999 12Zm0 0h7.5"
                    />
                  </svg>
                  Enqueue
                }
              </button>
            </form>
          </div>
        </section>
      </main>
    </div>
  `,
})
export class Messages {
  protected auth = inject(AuthService);
  private queue = inject(QueueService);
  private router = inject(Router);

  ackEnabled = signal(false);

  nackOpenId = signal<string | null>(null);
  nackError = signal('');
  copiedId = signal<string | null>(null);

  private requeueTrigger$ = new Subject<string>();
  private nackTrigger$ = new Subject<{ id: string; error: string }>();

  allMessages = signal<QueueMessage[]>([]);
  messagesTotal = signal(0);
  loadingMessages = signal(false);
  messagesError = signal('');

  private readonly PAGE_SIZE = PAGE_SIZE;
  private currentOffset = 0;
  private currentTopic: string | undefined;

  private enqueueModel = signal<EnqueueModel>({ topic: '', payload: '' });

  enqueueForm = form(
    this.enqueueModel,
    schema<EnqueueModel>((root) => {
      required(root.topic);
      required(root.payload);
    }),
  );

  private enqueueTrigger$ = new Subject<EnqueueRequest>();

  private enqueueState = toSignal(
    this.enqueueTrigger$.pipe(
      switchMap((req) =>
        this.queue.enqueueMessage(req).pipe(
          tap(() => {
            this.enqueueModel.set({ topic: '', payload: '' });
            this.metadataRows.set([]);
            this.loadMessages();
          }),
          map(
            (resp) =>
              ({ id: resp.id, loading: false, error: '' }) as EnqueueState,
          ),
          catchError((err) =>
            of({
              id: '',
              loading: false,
              error: err.error?.error || 'Failed to enqueue message',
            } as EnqueueState),
          ),
          startWith({ id: '', loading: true, error: '' } as EnqueueState),
        ),
      ),
    ),
    { initialValue: { id: '', loading: false, error: '' } },
  );

  enqueueSuccess = computed(() => this.enqueueState().id);
  enqueueError = computed(() => this.enqueueState().error);
  enqueueLoading = computed(() => this.enqueueState().loading);

  private requeueState = toSignal(
    this.requeueTrigger$.pipe(
      switchMap((id) =>
        this.queue.requeueMessage(id).pipe(
          tap(() => this.loadMessages()),
          map(() => ({ loading: false, error: '' })),
          catchError((err) =>
            of({ loading: false, error: err.error?.error || 'Failed to requeue' }),
          ),
          startWith({ loading: true, error: '' }),
        ),
      ),
    ),
    { initialValue: { loading: false, error: '' } },
  );

  private nackState = toSignal(
    this.nackTrigger$.pipe(
      switchMap(({ id, error }) =>
        this.queue.nackMessage(id, error).pipe(
          tap(() => {
            this.nackOpenId.set(null);
            this.nackError.set('');
            this.loadMessages();
          }),
          map(() => ({ loading: false, error: '' })),
          catchError((err) =>
            of({ loading: false, error: err.error?.error || 'Failed to nack' }),
          ),
          startWith({ loading: true, error: '' }),
        ),
      ),
    ),
    { initialValue: { loading: false, error: '' } },
  );

  private filterModel = signal('');
  filterForm = form(this.filterModel);

  metadataRows = signal<
    {
      model: ReturnType<typeof signal<MetadataRowModel>>;
      form: ReturnType<typeof form<MetadataRowModel>>;
    }[]
  >([]);

  objectKeys = Object.keys;

  trackByMsgId(_: number, msg: QueueMessage): string {
    return msg.id;
  }

  constructor() {
    this.loadMessages();
  }

  statusClasses(status: string): string {
    const base = 'inline-flex px-2 py-0.5 text-xs font-medium rounded-full';
    if (status === 'pending') return `${base} bg-yellow-100 text-yellow-800`;
    if (status === 'processing') return `${base} bg-blue-100 text-blue-800`;
    if (status === 'failed') return `${base} bg-red-100 text-red-800`;
    if (status === 'expired') return `${base} bg-orange-100 text-orange-800`;
    return `${base} bg-gray-100 text-gray-800`;
  }

  isDlq(msg: QueueMessage): boolean {
    return msg.topic.endsWith('.dlq');
  }

  retriesExhausted(msg: QueueMessage): boolean {
    return msg.retry_count >= msg.max_retries && msg.max_retries > 0;
  }

  rowClasses(msg: QueueMessage): string {
    const base = 'hover:bg-gray-50';
    if (this.isDlq(msg)) return `${base} bg-amber-50`;
    return base;
  }

  onRequeue(id: string) {
    this.requeueTrigger$.next(id);
  }

  onNackConfirm(id: string) {
    this.nackTrigger$.next({ id, error: this.nackError() });
  }

  loadMessages(reset = true): void {
    const topic = this.filterForm().value() || undefined;
    if (reset) {
      this.currentOffset = 0;
      this.currentTopic = topic;
      this.allMessages.set([]);
      this.messagesTotal.set(0);
    }
    if (this.loadingMessages()) return;
    this.loadingMessages.set(true);
    this.messagesError.set('');

    this.queue.listMessages(this.currentTopic, this.currentOffset).subscribe({
      next: (page) => {
        this.allMessages.update((msgs) => [...msgs, ...page.items]);
        this.messagesTotal.set(page.total);
        this.currentOffset += page.items.length;
        this.loadingMessages.set(false);
      },
      error: (err: { error?: { error?: string } }) => {
        this.messagesError.set(err.error?.error ?? 'Failed to load messages');
        this.loadingMessages.set(false);
      },
    });
  }

  onScrollIndexChange(index: number): void {
    const loaded = this.allMessages().length;
    const total = this.messagesTotal();
    if (loaded > 0 && index >= loaded - 15 && loaded < total) {
      this.loadMessages(false);
    }
  }

  onEnqueue(event: Event) {
    event.preventDefault();
    const metadata: Record<string, string> = {};
    for (const row of this.metadataRows()) {
      const key = row.form.key().value().trim();
      if (key) {
        metadata[key] = row.form.value().value();
      }
    }

    this.enqueueTrigger$.next({
      topic: this.enqueueForm.topic().value(),
      payload: this.enqueueForm.payload().value(),
      metadata,
    });
  }

  addMetadataRow() {
    const model = signal<MetadataRowModel>({ key: '', value: '' });
    this.metadataRows.update((rows) => [...rows, { model, form: form(model) }]);
  }

  removeMetadataRow(index: number) {
    this.metadataRows.update((rows) => rows.filter((_, i) => i !== index));
  }

  copyId(id: string): void {
    navigator.clipboard.writeText(id).then(() => {
      this.copiedId.set(id);
      setTimeout(() => this.copiedId.set(null), 1500);
    });
  }

  onLogout() {
    this.auth.logout();
    this.router.navigate(['/login']);
  }
}
