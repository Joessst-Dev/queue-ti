import { vi } from 'vitest';

vi.mock('chart.js', () => ({
  Chart: class {
    static register = vi.fn();
    update = vi.fn();
    destroy = vi.fn();
  },
  BarController: {},
  BarElement: {},
  CategoryScale: {},
  LinearScale: {},
  Tooltip: {},
  Legend: {},
}));

import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of } from 'rxjs';
import { QueueStatsChart } from './queue-stats-chart';
import { QueueService, StatsResponse } from '../services/queue.service';
import { AuthService } from '../services/auth.service';

const makeQueueService = (response: StatsResponse) =>
  ({
    getStats: vi.fn().mockReturnValue(of(response)),
  }) as unknown as QueueService;

const makeAuthService = () =>
  ({
    isAuthenticated: vi.fn().mockReturnValue(false),
    getAuthHeader: vi.fn().mockReturnValue(null),
  }) as unknown as AuthService;

const setup = async (response: StatsResponse = { topics: [] }) => {
  const queueService = makeQueueService(response);
  const authService = makeAuthService();

  await TestBed.configureTestingModule({
    imports: [QueueStatsChart],
    providers: [
      provideZonelessChangeDetection(),
      { provide: QueueService, useValue: queueService },
      { provide: AuthService, useValue: authService },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(QueueStatsChart);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  return { fixture, component: fixture.componentInstance, queueService };
};

describe('QueueStatsChart', () => {
  describe('on init', () => {
    it('should call getStats() immediately', async () => {
      const { queueService } = await setup();
      expect(queueService.getStats).toHaveBeenCalled();
    });
  });

  describe('when topics is empty', () => {
    it('should show the empty state message', async () => {
      const { fixture } = await setup({ topics: [] });

      const el: HTMLElement = fixture.nativeElement;
      expect(el.textContent).toContain('No messages in any topic.');
    });
  });

  describe('when topics are returned', () => {
    it('should NOT show the empty state message', async () => {
      const { fixture } = await setup({
        topics: [
          { topic: 'orders', status: 'pending', count: 42 },
          { topic: 'orders', status: 'processing', count: 3 },
        ],
      });

      const el: HTMLElement = fixture.nativeElement;
      expect(el.textContent).not.toContain('No messages in any topic.');
    });
  });
});
