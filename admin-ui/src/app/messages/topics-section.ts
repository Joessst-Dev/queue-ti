import { Component, ChangeDetectionStrategy } from '@angular/core';
import { TopicConfigSection } from './topic-config-section';
import { TopicSchemaSection } from './topic-schema-section';

@Component({
  selector: 'app-topics-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TopicConfigSection, TopicSchemaSection],
  template: `
    <div class="space-y-6">
      <app-topic-config-section />
      <app-topic-schema-section />
    </div>
  `,
})
export class TopicsSection {}
