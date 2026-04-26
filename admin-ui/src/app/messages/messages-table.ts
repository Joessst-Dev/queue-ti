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
        </div>
      </div>

      @if (error()) {
        <div class="px-6 py-4 text-sm text-red-600">
          {{ error() }}
        </div>
      }

      <div class="overflow-x-auto">
        <!-- Header table — lives outside the viewport so it never gets translated away -->
        <table
          class="w-full table-fixed border-b border-gray-200"
          [style.width]="'calc(100% - ' + scrollbarWidth() + 'px)'"
        >
          <colgroup>
            <col style="width: 10%">
            <col style="width: 13%">
            <col style="width: 20%">
            <col style="width: 9%">
            <col style="width: 8%">
            <col style="width: 10%">
            <col style="width: 13%">
            <col style="width: 10%">
            <col style="width: 7%">
          </colgroup>
          <thead class="bg-gray-50">
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
        </table>
        <cdk-virtual-scroll-viewport
          [itemSize]="73"
          style="height: 520px;"
          (scrolledIndexChange)="scrollIndexChange.emit($event)"
        >
          <table class="w-full table-fixed">
            <colgroup>
              <col style="width: 10%">
              <col style="width: 13%">
              <col style="width: 20%">
              <col style="width: 9%">
              <col style="width: 8%">
              <col style="width: 10%">
              <col style="width: 13%">
              <col style="width: 10%">
              <col style="width: 7%">
            </colgroup>
            <tbody class="divide-y divide-gray-200">
              <tr *cdkVirtualFor="let msg of messages(); trackBy: trackByMsgId" [class]="rowClasses(msg)">
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
                      (click)="requeue.emit(msg.id)"
                      class="px-3 py-1 text-xs font-medium bg-amber-100 text-amber-800 rounded hover:bg-amber-200 cursor-pointer"
                    >
                      Requeue
                    </button>
                  } @else if (msg.status === 'processing') {
                    @if (nackOpenId() === msg.id) {
                      <div class="flex flex-col gap-1">
                        <input
                          type="text"
                          [value]="nackError()"
                          (input)="nackError.set($any($event.target).value)"
                          placeholder="Error reason (optional)"
                          class="px-2 py-1 text-xs border border-gray-300 rounded focus:outline-none focus:ring-1 focus:ring-red-400 w-full"
                        />
                        <div class="flex items-center gap-1">
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
              @if (messages().length === 0 && !loading()) {
                <tr>
                  <td
                    colspan="9"
                    class="px-6 py-12 text-center text-sm text-gray-500"
                  >
                    No messages found
                  </td>
                </tr>
              }
              @if (messages().length === 0 && loading()) {
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

  private readonly viewport = viewChild(CdkVirtualScrollViewport);

  readonly scrollbarWidth = signal(0);
  readonly nackOpenId = signal<string | null>(null);
  readonly nackError = signal('');
  readonly copiedId = signal<string | null>(null);

  private readonly filterModel = signal('');
  readonly filterForm = form(this.filterModel);

  readonly objectKeys = Object.keys;

  constructor() {
    afterEveryRender(() => {
      const el = this.viewport()?.elementRef.nativeElement;
      if (el) this.scrollbarWidth.set(el.offsetWidth - el.clientWidth);
    });
  }

  trackByMsgId(_: number, msg: QueueMessage): string {
    return msg.id;
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
}
