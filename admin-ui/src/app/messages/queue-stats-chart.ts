import {
  Component,
  OnDestroy,
  inject,
  signal,
  viewChild,
  ElementRef,
  afterNextRender,
  ChangeDetectionStrategy,
} from '@angular/core';
import {
  Chart,
  BarController,
  BarElement,
  CategoryScale,
  LinearScale,
  Tooltip,
  Legend,
} from 'chart.js';
import { DatePipe } from '@angular/common';
import { interval, Subscription, startWith, switchMap } from 'rxjs';
import { QueueService, TopicStat } from '../services/queue.service';

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Tooltip, Legend);

const STATUS_ORDER = ['pending', 'processing', 'failed', 'expired'] as const;

const STATUS_COLORS: Record<string, string> = {
  pending: 'rgba(251, 191, 36, 0.8)',
  processing: 'rgba(59, 130, 246, 0.8)',
  failed: 'rgba(239, 68, 68, 0.8)',
  expired: 'rgba(249, 115, 22, 0.8)',
};

const DEFAULT_COLOR = 'rgba(107, 114, 128, 0.8)';

function colorForStatus(status: string): string {
  return STATUS_COLORS[status] ?? DEFAULT_COLOR;
}

function orderedStatuses(statuses: string[]): string[] {
  const known = STATUS_ORDER.filter((s) => statuses.includes(s));
  const others = statuses
    .filter((s) => !(STATUS_ORDER as readonly string[]).includes(s))
    .sort();
  return [...known, ...others];
}

@Component({
  selector: 'app-queue-stats-chart',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe],
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
              d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 0 1 3 19.875v-6.75ZM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V8.625ZM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V4.125Z"
            />
          </svg>
          Queue Depth
          @if (lastUpdated()) {
            <span class="ml-auto text-xs font-normal text-gray-400">
              Updated {{ lastUpdated() | date: 'HH:mm:ss' }}
            </span>
          }
        </h2>
      </div>
      <div class="px-6 py-4">
        <canvas #chartCanvas aria-label="Queue depth stacked bar chart" role="img"></canvas>
      </div>
      @if (isEmpty()) {
        <p class="px-6 pb-4 text-sm text-gray-400 text-center">No messages in any topic.</p>
      }
    </section>
  `,
})
export class QueueStatsChart implements OnDestroy {
  private readonly queue = inject(QueueService);

  private chart: Chart | null = null;
  private sub: Subscription | null = null;

  readonly isEmpty = signal(false);
  readonly lastUpdated = signal<Date | null>(null);

  private readonly chartCanvas = viewChild<ElementRef<HTMLCanvasElement>>('chartCanvas');

  constructor() {
    afterNextRender(() => {
      this.startPolling();
    });
  }

  private startPolling(): void {
    this.sub = interval(30_000)
      .pipe(
        startWith(0),
        switchMap(() => this.queue.getStats()),
      )
      .subscribe((resp) => {
        this.renderChart(resp.topics);
        this.lastUpdated.set(new Date());
        this.isEmpty.set(resp.topics.length === 0);
      });
  }

  private renderChart(stats: TopicStat[]): void {
    const topics = [...new Map(stats.map((s) => [s.topic, true])).keys()];
    const allStatuses = [...new Set(stats.map((s) => s.status))];
    const statuses = orderedStatuses(allStatuses);

    const countMap = new Map<string, number>();
    for (const s of stats) {
      countMap.set(`${s.topic}::${s.status}`, s.count);
    }

    const datasets = statuses.map((status) => ({
      label: status,
      data: topics.map((topic) => countMap.get(`${topic}::${status}`) ?? 0),
      backgroundColor: colorForStatus(status),
    }));

    if (this.chart !== null) {
      this.chart.data.labels = topics;
      this.chart.data.datasets = datasets;
      this.chart.update();
      return;
    }

    const canvas = this.chartCanvas()?.nativeElement;
    if (!canvas) return;

    this.chart = new Chart(canvas, {
      type: 'bar',
      data: { labels: topics, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio: 3,
        plugins: {
          legend: { position: 'bottom' },
          tooltip: { mode: 'index', intersect: false },
        },
        scales: {
          x: { stacked: true },
          y: {
            stacked: true,
            beginAtZero: true,
            ticks: { stepSize: 1, precision: 0 },
          },
        },
      },
    });
  }

  ngOnDestroy(): void {
    this.sub?.unsubscribe();
    this.chart?.destroy();
  }
}
