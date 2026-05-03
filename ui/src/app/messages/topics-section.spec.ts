import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection, Component, input } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { TopicsSection } from './topics-section';
import { TopicConfigSection } from './topic-config-section';
import { TopicSchemaSection } from './topic-schema-section';
import { ConsumerGroupsSection } from './consumer-groups-section/consumer-groups-section.component';
import { QueueService, TopicConfigsResponse } from '../services/queue.service';

@Component({ selector: 'app-topic-config-section', template: '<div>topic-config</div>', standalone: true })
class StubTopicConfigSection {}

@Component({ selector: 'app-topic-schema-section', template: '<div>topic-schema</div>', standalone: true })
class StubTopicSchemaSection {}

@Component({ selector: 'app-consumer-groups-section', template: '<div>consumer-groups</div>', standalone: true })
class StubConsumerGroupsSection {
  readonly topic = input.required<string>();
}

const makeQueueService = (topics: string[] = [], fail = false): QueueService => {
  const response: TopicConfigsResponse = {
    items: topics.map((t) => ({
      topic: t,
      max_retries: null,
      message_ttl_seconds: null,
      max_depth: null,
      replayable: false,
      replay_window_seconds: null,
      throughput_limit: null,
    })),
  };
  return {
    getTopicConfigs: vi.fn().mockReturnValue(
      fail ? throwError(() => new Error('network error')) : of(response),
    ),
  } as unknown as QueueService;
};

const setup = async (topics: string[] = [], fail = false) => {
  const queueService = makeQueueService(topics, fail);

  await TestBed.configureTestingModule({
    imports: [TopicsSection],
    providers: [
      provideZonelessChangeDetection(),
      { provide: QueueService, useValue: queueService },
    ],
  })
    .overrideComponent(TopicsSection, {
      remove: { imports: [TopicConfigSection, TopicSchemaSection, ConsumerGroupsSection] },
      add: { imports: [StubTopicConfigSection, StubTopicSchemaSection, StubConsumerGroupsSection] },
    })
    .compileComponents();

  const fixture = TestBed.createComponent(TopicsSection);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  const el: HTMLElement = fixture.nativeElement;
  return { fixture, component: fixture.componentInstance, queueService, el };
};

describe('TopicsSection', () => {
  describe('when rendered', () => {
    it('should render topic config section', async () => {
      const { el } = await setup();
      expect(el.querySelector('app-topic-config-section')).not.toBeNull();
    });

    it('should render topic schema section', async () => {
      const { el } = await setup();
      expect(el.querySelector('app-topic-schema-section')).not.toBeNull();
    });

    it('should render a topic selector', async () => {
      const { el } = await setup();
      expect(el.querySelector('select[aria-label="Select topic for consumer groups"]')).not.toBeNull();
    });

    it('should not render consumer groups section when no topic is selected', async () => {
      const { el } = await setup();
      expect(el.querySelector('app-consumer-groups-section')).toBeNull();
    });
  });

  describe('when topics are loaded', () => {
    it('should populate the select with available topics', async () => {
      const { el } = await setup(['orders', 'payments']);
      const options = el.querySelectorAll('select option');
      // first option is the placeholder, then one per topic
      expect(options.length).toBe(3);
      expect(options[1].textContent?.trim()).toBe('orders');
      expect(options[2].textContent?.trim()).toBe('payments');
    });
  });

  describe('when a topic is selected', () => {
    it('should render the consumer groups section', async () => {
      const { fixture, el, component } = await setup(['orders']);
      component.selectedTopic.set('orders');
      fixture.detectChanges();
      await fixture.whenStable();
      expect(el.querySelector('app-consumer-groups-section')).not.toBeNull();
    });
  });

  describe('when topic load fails', () => {
    it('should display an error banner', async () => {
      const { el } = await setup([], true);
      expect(el.textContent).toContain('Failed to load topics');
    });
  });
});
