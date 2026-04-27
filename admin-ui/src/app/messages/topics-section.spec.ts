import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection, Component } from '@angular/core';
import { TopicsSection } from './topics-section';
import { TopicConfigSection } from './topic-config-section';
import { TopicSchemaSection } from './topic-schema-section';

@Component({ selector: 'app-topic-config-section', template: '<div>topic-config</div>', standalone: true })
class StubTopicConfigSection {}

@Component({ selector: 'app-topic-schema-section', template: '<div>topic-schema</div>', standalone: true })
class StubTopicSchemaSection {}

const setup = async () => {
  await TestBed.configureTestingModule({
    imports: [TopicsSection],
    providers: [provideZonelessChangeDetection()],
  })
    .overrideComponent(TopicsSection, {
      remove: { imports: [TopicConfigSection, TopicSchemaSection] },
      add: { imports: [StubTopicConfigSection, StubTopicSchemaSection] },
    })
    .compileComponents();

  const fixture = TestBed.createComponent(TopicsSection);
  fixture.detectChanges();
  await fixture.whenStable();
  return { fixture };
};

describe('TopicsSection', () => {
  describe('when rendered', () => {
    it('should render topic config section', async () => {
      const { fixture } = await setup();
      expect(fixture.nativeElement.querySelector('app-topic-config-section')).not.toBeNull();
    });

    it('should render topic schema section', async () => {
      const { fixture } = await setup();
      expect(fixture.nativeElement.querySelector('app-topic-schema-section')).not.toBeNull();
    });
  });
});
