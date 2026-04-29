import { Component, inject, ChangeDetectionStrategy } from '@angular/core';
import { VersionService } from '../services/version.service';

@Component({
  selector: 'app-footer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <footer class="py-4 text-center text-xs text-gray-400">
      queue-ti {{ version() }}
    </footer>
  `,
})
export class FooterComponent {
  private readonly versionService = inject(VersionService);
  protected readonly version = this.versionService.version;
}
