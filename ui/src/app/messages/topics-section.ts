import { Component, ChangeDetectionStrategy } from '@angular/core';
import { TopicConfigSection } from './topic-config-section';
import { TopicSchemaSection } from './topic-schema-section';

@Component({
  selector: 'app-topics-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TopicConfigSection, TopicSchemaSection],
  template: `
    <div class="space-y-6">
      <div><app-topic-config-section /></div>
      <div><app-topic-schema-section /></div>
    </div>
  `,
})
export class TopicsSection {}
