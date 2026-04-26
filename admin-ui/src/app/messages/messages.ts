import { Component, inject, computed, signal, ChangeDetectionStrategy } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { Router } from '@angular/router';
import { Subject, switchMap, map, catchError, of, startWith, tap } from 'rxjs';
import { AuthService } from '../services/auth.service';
import {
  QueueService,
  QueueMessage,
  EnqueueRequest,
  PAGE_SIZE,
} from '../services/queue.service';
import { MessagesHeader } from './messages-header';
import { MessagesTable } from './messages-table';
import { EnqueueSection } from './enqueue-section';
import { QueueStatsChart } from './queue-stats-chart';

interface EnqueueState {
  id: string;
  loading: boolean;
  error: string;
}

@Component({
  selector: 'app-messages',
  imports: [MessagesHeader, MessagesTable, EnqueueSection, QueueStatsChart],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="min-h-screen bg-gray-50">
      <app-messages-header [isAuthenticated]="auth.isAuthenticated()" (signOut)="onLogout()" />

      <div class="bg-white border-b border-gray-200">
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <nav class="-mb-px flex gap-6" aria-label="Main navigation">
            @for (tab of tabs; track tab.id) {
              <button
                (click)="activeTab.set(tab.id)"
                [class]="activeTab() === tab.id
                  ? 'border-indigo-500 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'"
                class="border-b-2 py-3 px-1 text-sm font-medium whitespace-nowrap cursor-pointer transition-colors"
              >
                {{ tab.label }}
              </button>
            }
          </nav>
        </div>
      </div>

      <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        @if (activeTab() === 'messages') {
          <app-messages-table
            [messages]="allMessages()"
            [loading]="loadingMessages()"
            [error]="messagesError()"
            (topicSearch)="onTopicSearch($event)"
            (scrollIndexChange)="onScrollIndexChange($event)"
            (requeue)="onRequeue($event)"
            (nackConfirm)="onNackConfirm($event)"
          />
        }
        @if (activeTab() === 'enqueue') {
          <app-enqueue-section
            [success]="enqueueSuccess()"
            [error]="enqueueError()"
            [loading]="enqueueLoading()"
            (enqueue)="onEnqueueRequest($event)"
          />
        }
        @if (activeTab() === 'stats') {
          <app-queue-stats-chart />
        }
      </main>
    </div>
  `,
})
export class Messages {
  protected readonly auth = inject(AuthService);
  private readonly queue = inject(QueueService);
  private readonly router = inject(Router);

  readonly activeTab = signal<'messages' | 'enqueue' | 'stats'>('messages');
  readonly tabs = [
    { id: 'messages' as const, label: 'Messages' },
    { id: 'enqueue' as const, label: 'Enqueue' },
    { id: 'stats'    as const, label: 'Stats'    },
  ];

  private readonly requeueTrigger$ = new Subject<string>();
  private readonly nackTrigger$ = new Subject<{ id: string; error: string }>();
  private readonly enqueueTrigger$ = new Subject<EnqueueRequest>();

  readonly allMessages = signal<QueueMessage[]>([]);
  readonly messagesTotal = signal(0);
  readonly loadingMessages = signal(false);
  readonly messagesError = signal('');

  private readonly PAGE_SIZE = PAGE_SIZE;
  private currentOffset = 0;
  private currentTopic: string | undefined;

  private readonly enqueueState = toSignal(
    this.enqueueTrigger$.pipe(
      switchMap((req) =>
        this.queue.enqueueMessage(req).pipe(
          tap(() => this.loadMessages()),
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

  readonly enqueueSuccess = computed(() => this.enqueueState().id);
  readonly enqueueError = computed(() => this.enqueueState().error);
  readonly enqueueLoading = computed(() => this.enqueueState().loading);

  private readonly requeueState = toSignal(
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

  private readonly nackState = toSignal(
    this.nackTrigger$.pipe(
      switchMap(({ id, error }) =>
        this.queue.nackMessage(id, error).pipe(
          tap(() => this.loadMessages()),
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

  constructor() {
    this.loadMessages();
  }

  loadMessages(reset = true): void {
    if (reset) {
      this.currentOffset = 0;
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

  onTopicSearch(topic: string): void {
    this.currentTopic = topic || undefined;
    this.loadMessages(true);
  }

  onScrollIndexChange(index: number): void {
    const loaded = this.allMessages().length;
    const total = this.messagesTotal();
    if (loaded > 0 && index >= loaded - 15 && loaded < total) {
      this.loadMessages(false);
    }
  }

  onRequeue(id: string): void {
    this.requeueTrigger$.next(id);
  }

  onNackConfirm({ id, error }: { id: string; error: string }): void {
    this.nackTrigger$.next({ id, error });
  }

  onEnqueueRequest(req: EnqueueRequest): void {
    this.enqueueTrigger$.next(req);
  }

  onLogout(): void {
    this.auth.logout();
    this.router.navigate(['/login']);
  }
}
