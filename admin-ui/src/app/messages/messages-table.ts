import {
  Component,
  input,
  output,
  signal,
  viewChild,
  afterEveryRender,
  ChangeDetectionStrategy,
} from '@angular/core';
import { FormField, form } from '@angular/forms/signals';
import { SlicePipe, DatePipe } from '@angular/common';
import {
  CdkVirtualScrollViewport,
  CdkVirtualForOf,
  CdkFixedSizeVirtualScroll,
} from '@angular/cdk/scrolling';
import { QueueMessage } from '../services/queue.service';

// py-4 top (16) + text-sm line-height (20) + py-4 bottom (16) + divide-y border (1) = 53
const ITEM_SIZE = 53;

@Component({
  selector: 'app-messages-table',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormField, SlicePipe, DatePipe, CdkVirtualScrollViewport, CdkVirtualForOf, CdkFixedSizeVirtualScroll],
  template: `
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
            (keydown.enter)="onRefresh()"
            class="px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
          />
          <button
            (click)="onRefresh()"
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
          @if (filterForm().value()) {
            <button
              type="button"
              (click)="showPurgeConfirm.set(true)"
              class="px-3 py-1.5 text-sm font-medium text-red-600 border border-red-300 rounded-md hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-red-400 cursor-pointer"
            >
              Purge
            </button>
          }
        </div>
      </div>

      @if (showPurgeConfirm()) {
        <div class="px-6 py-3 border-b border-red-200 bg-red-50 space-y-2">
          <p class="text-sm font-medium text-red-800">Purge messages from "{{ filterForm().value() }}"</p>
          <div class="flex gap-3 text-sm">
            @for (status of ['pending', 'processing', 'expired']; track status) {
              <label class="flex items-center gap-1 cursor-pointer">
                <input
                  type="checkbox"
                  [checked]="purgeStatuses().includes(status)"
                  (change)="togglePurgeStatus(status)"
                />
                {{ status }}
              </label>
            }
          </div>
          <div class="flex gap-2">
            <button
              type="button"
              [disabled]="purgeStatuses().length === 0"
              (click)="confirmPurge()"
              class="px-3 py-1.5 text-sm font-medium text-white bg-red-600 rounded-md hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Confirm purge
            </button>
            <button
              type="button"
              (click)="showPurgeConfirm.set(false)"
              class="px-3 py-1.5 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
            >
              Cancel
            </button>
          </div>
        </div>
      }

      @if (error()) {
        <div class="px-6 py-4 text-sm text-red-600">
          {{ error() }}
        </div>
      }

      <div class="overflow-x-auto">
        <!-- Header table — lives outside the viewport so it never scrolls away -->
        <table
          class="w-full table-fixed border-b border-gray-200"
          [style.width]="'calc(100% - ' + scrollbarWidth() + 'px)'"
        >
          <colgroup>
            <col class="hidden lg:table-column" style="width: 9%">
            <col style="width: 13%">
            <col class="hidden md:table-column" style="width: 10%">
            <col class="hidden md:table-column" style="width: 16%">
            <col style="width: 8%">
            <col class="hidden md:table-column" style="width: 7%">
            <col class="hidden lg:table-column" style="width: 9%">
            <col class="hidden lg:table-column" style="width: 11%">
            <col class="hidden md:table-column" style="width: 9%">
            <col style="width: 8%">
          </colgroup>
          <thead class="bg-gray-50">
            <tr>
              <th class="hidden lg:table-cell px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">ID</th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Topic</th>
              <th class="hidden md:table-cell px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Key</th>
              <th class="hidden md:table-cell px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Payload</th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
              <th class="hidden md:table-cell px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Retries</th>
              <th class="hidden lg:table-cell px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Expires</th>
              <th class="hidden lg:table-cell px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Metadata</th>
              <th class="hidden md:table-cell px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
              <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
            </tr>
          </thead>
        </table>

        <cdk-virtual-scroll-viewport
          [itemSize]="itemSize"
          [minBufferPx]="600"
          [maxBufferPx]="1200"
          [style.height]="viewportHeight()"
          (scrolledIndexChange)="scrollIndexChange.emit($event)"
        >
          <table class="w-full table-fixed">
            <colgroup>
              <col class="hidden lg:table-column" style="width: 9%">
              <col style="width: 13%">
              <col class="hidden md:table-column" style="width: 10%">
              <col class="hidden md:table-column" style="width: 16%">
              <col style="width: 8%">
              <col class="hidden md:table-column" style="width: 7%">
              <col class="hidden lg:table-column" style="width: 9%">
              <col class="hidden lg:table-column" style="width: 11%">
              <col class="hidden md:table-column" style="width: 9%">
              <col style="width: 8%">
            </colgroup>
            <tbody class="divide-y divide-gray-200">
              <tr *cdkVirtualFor="let msg of messages(); trackBy: trackByMsgId" [class]="rowClasses(msg)">

                <!-- ID -->
                <td class="hidden lg:table-cell px-6 py-4 text-sm font-mono text-gray-600 overflow-hidden">
                  <div class="flex items-center gap-1 min-w-0">
                    <span class="truncate" [title]="msg.id">{{ msg.id | slice: 0 : 8 }}&hellip;</span>
                    <button
                      type="button"
                      (click)="copyId(msg.id)"
                      [title]="'Copy full ID: ' + msg.id"
                      class="shrink-0 text-gray-400 hover:text-gray-600 cursor-pointer"
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

                <!-- Topic — ellipsis + tooltip; original_topic in tooltip only -->
                <td
                  class="px-6 py-4 text-sm text-gray-900 truncate"
                  [title]="msg.original_topic ? msg.topic + ' (from: ' + msg.original_topic + ')' : msg.topic"
                >
                  {{ msg.topic }}
                </td>

                <!-- Key -->
                <td class="hidden md:table-cell px-6 py-4 text-sm text-gray-600 font-mono truncate" [title]="msg.key ?? ''">
                  @if (msg.key) {
                    {{ msg.key }}
                  } @else {
                    <span class="text-gray-400">&mdash;</span>
                  }
                </td>

                <!-- Payload -->
                <td class="hidden md:table-cell px-6 py-4 text-sm text-gray-600 font-mono truncate" [title]="msg.payload">
                  {{ msg.payload }}
                </td>

                <!-- Status -->
                <td class="px-6 py-4 text-sm">
                  <span
                    class="inline-flex px-2 py-0.5 text-xs font-medium rounded-full"
                    [class]="statusClasses(msg.status)"
                  >
                    {{ msg.status }}
                  </span>
                </td>

                <!-- Retries -->
                <td class="hidden md:table-cell px-6 py-4 text-sm">
                  <span
                    [title]="msg.last_error || ''"
                    [class]="retriesExhausted(msg) ? 'text-red-600 font-medium' : 'text-gray-500'"
                  >
                    {{ msg.retry_count }} / {{ msg.max_retries }}
                  </span>
                </td>

                <!-- Expires -->
                <td class="hidden lg:table-cell px-6 py-4 text-sm text-gray-500 truncate">
                  @if (msg.expires_at) {
                    {{ msg.expires_at | date: 'short' }}
                  } @else {
                    <span class="text-gray-400">&mdash;</span>
                  }
                </td>

                <!-- Metadata — single-line, clips overflow tags -->
                <td class="hidden lg:table-cell px-6 py-4 text-sm text-gray-500 overflow-hidden"
                    [title]="metadataTitle(msg.metadata)">
                  @if (msg.metadata && objectKeys(msg.metadata).length > 0) {
                    <div class="flex gap-1 overflow-hidden">
                      @for (key of objectKeys(msg.metadata); track key) {
                        <span class="shrink-0 inline-flex items-center px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-700">
                          {{ key }}={{ msg.metadata[key] }}
                        </span>
                      }
                    </div>
                  } @else {
                    <span class="text-gray-400">&mdash;</span>
                  }
                </td>

                <!-- Created -->
                <td class="hidden md:table-cell px-6 py-4 text-sm text-gray-500 truncate">
                  {{ msg.created_at | date: 'short' }}
                </td>

                <!-- Actions — single row of compact buttons -->
                <td class="px-6 py-4 text-sm overflow-visible">
                  <div class="flex items-center gap-1 flex-nowrap">
                    @if (isDlq(msg)) {
                      <button
                        (click)="requeue.emit(msg.id)"
                        class="px-2 py-0.5 text-xs font-medium bg-amber-100 text-amber-800 rounded hover:bg-amber-200 cursor-pointer whitespace-nowrap"
                      >
                        Requeue
                      </button>
                    } @else if (msg.status === 'processing') {
                      @if (nackOpenId() === msg.id) {
                        <input
                          type="text"
                          [value]="nackError()"
                          (input)="nackError.set($any($event.target).value)"
                          placeholder="Reason…"
                          class="px-1 py-0.5 text-xs border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-red-400 w-20 shrink-0"
                        />
                        <button
                          (click)="onNackConfirm(msg.id)"
                          title="Confirm nack"
                          class="shrink-0 px-1.5 py-0.5 text-xs font-medium bg-red-100 text-red-800 rounded hover:bg-red-200 cursor-pointer"
                        >
                          ✓
                        </button>
                        <button
                          (click)="nackOpenId.set(null)"
                          title="Cancel"
                          class="shrink-0 px-1.5 py-0.5 text-xs text-gray-500 hover:text-gray-700 cursor-pointer"
                        >
                          ✗
                        </button>
                      } @else {
                        <button
                          (click)="nackOpenId.set(msg.id); nackError.set('')"
                          class="px-2 py-0.5 text-xs font-medium bg-red-100 text-red-800 rounded hover:bg-red-200 cursor-pointer whitespace-nowrap"
                        >
                          Nack
                        </button>
                      }
                    }
                    @if (msg.key) {
                      <button
                        type="button"
                        (click)="onPurgeByKey(msg)"
                        [title]="purgeKeyTitle(msg.key)"
                        class="shrink-0 p-0.5 text-red-400 hover:text-red-600 cursor-pointer"
                      >
                        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" d="m14.74 9-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 0 1-2.244 2.077H8.084a2.25 2.25 0 0 1-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 0 0-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 0 1 3.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 0 0-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 0 0-7.5 0" />
                        </svg>
                      </button>
                    }
                  </div>
                </td>

              </tr>
              @if (messages().length === 0 && !loading()) {
                <tr>
                  <td colspan="10" class="px-6 py-12 text-center text-sm text-gray-500">
                    No messages found
                  </td>
                </tr>
              }
              @if (messages().length === 0 && loading()) {
                <tr>
                  <td colspan="10" class="px-6 py-12 text-center text-sm text-gray-500">
                    Loading messages...
                  </td>
                </tr>
              }
            </tbody>
          </table>
        </cdk-virtual-scroll-viewport>
      </div>
    </section>
  `,
})
export class MessagesTable {
  readonly messages = input.required<QueueMessage[]>();
  readonly loading = input.required<boolean>();
  readonly error = input<string>('');

  readonly topicSearch = output<string>();
  readonly scrollIndexChange = output<number>();
  readonly requeue = output<string>();
  readonly nackConfirm = output<{ id: string; error: string }>();
  readonly purge = output<{ topic: string; statuses: string[] }>();
  readonly purgeByKey = output<{ topic: string; key: string }>();

  readonly itemSize = ITEM_SIZE;

  private readonly viewport = viewChild(CdkVirtualScrollViewport);
  readonly scrollbarWidth = signal(0);

  readonly nackOpenId = signal<string | null>(null);
  readonly nackError = signal('');
  readonly copiedId = signal<string | null>(null);
  readonly purgeStatuses = signal<string[]>(['pending', 'processing', 'expired']);
  readonly showPurgeConfirm = signal(false);

  private readonly filterModel = signal('');
  readonly filterForm = form(this.filterModel);

  readonly objectKeys = Object.keys;

  constructor() {
    afterEveryRender(() => {
      const el = this.viewport()?.elementRef.nativeElement;
      if (el) this.scrollbarWidth.set(el.offsetWidth - el.clientWidth);
    });
  }

  viewportHeight(): string {
    const count = this.messages().length;
    if (count === 0) return '120px';
    return `${Math.min(520, count * ITEM_SIZE)}px`;
  }

  trackByMsgId(_: number, msg: QueueMessage): string {
    return msg.id;
  }

  purgeKeyTitle(key: string | null | undefined): string {
    return key ? `Purge all messages with key "${key}"` : '';
  }

  metadataTitle(metadata: Record<string, string> | null | undefined): string {
    if (!metadata) return '';
    return Object.entries(metadata).map(([k, v]) => `${k}=${v}`).join(', ');
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

  onRefresh(): void {
    this.topicSearch.emit(this.filterForm().value() ?? '');
  }

  onNackConfirm(id: string): void {
    this.nackConfirm.emit({ id, error: this.nackError() });
    this.nackOpenId.set(null);
    this.nackError.set('');
  }

  copyId(id: string): void {
    navigator.clipboard.writeText(id).then(() => {
      this.copiedId.set(id);
      setTimeout(() => this.copiedId.set(null), 1500);
    });
  }

  togglePurgeStatus(status: string): void {
    this.purgeStatuses.update((current) =>
      current.includes(status)
        ? current.filter((s) => s !== status)
        : [...current, status],
    );
  }

  confirmPurge(): void {
    const topic = this.filterForm().value() ?? '';
    this.purge.emit({ topic, statuses: this.purgeStatuses() });
    this.showPurgeConfirm.set(false);
  }

  onPurgeByKey(msg: QueueMessage): void {
    if (!msg.key) return;
    const confirmed = confirm(`Purge all messages with key "${msg.key}" from topic "${msg.topic}"?`);
    if (confirmed) {
      this.purgeByKey.emit({ topic: msg.topic, key: msg.key });
    }
  }
}
