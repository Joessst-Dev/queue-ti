import { Component, inject, signal, OnInit, ChangeDetectionStrategy } from '@angular/core';
import { QueueService } from '../services/queue.service';
import { TopicConfigSection } from './topic-config-section';
import { TopicSchemaSection } from './topic-schema-section';
import { ConsumerGroupsSection } from './consumer-groups-section/consumer-groups-section.component';

@Component({
  selector: 'app-topics-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TopicConfigSection, TopicSchemaSection, ConsumerGroupsSection],
  template: `
    <div class="space-y-6">
      <div><app-topic-config-section /></div>
      <div><app-topic-schema-section /></div>

      <div class="bg-white shadow rounded-lg px-6 py-4 flex items-center gap-3">
        <label for="cg-topic-select" class="text-sm font-medium text-gray-700 whitespace-nowrap">
          Topic
        </label>
        <select
          id="cg-topic-select"
          [value]="selectedTopic()"
          (change)="selectedTopic.set(selectValue($event))"
          class="flex-1 max-w-xs px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
          aria-label="Select topic for consumer groups"
        >
          <option value="">— select a topic —</option>
          @for (t of topics(); track t) {
            <option [value]="t">{{ t }}</option>
          }
        </select>
      </div>

      @if (selectedTopic()) {
        <div>
          <app-consumer-groups-section [topic]="selectedTopic()" />
        </div>
      }
    </div>
  `,
})
export class TopicsSection implements OnInit {
  private readonly queue = inject(QueueService);

  readonly topics = signal<string[]>([]);
  readonly selectedTopic = signal('');

  ngOnInit(): void {
    this.queue.getTopicConfigs().subscribe({
      next: (res) => this.topics.set(res.items.map((c) => c.topic)),
      error: () => {},
    });
  }

  selectValue(e: Event): string {
    return (e.target as HTMLSelectElement).value;
  }
}
