import { Component, inject, ChangeDetectionStrategy } from '@angular/core';
import { VersionService } from '../services/version.service';

@Component({
  selector: 'app-footer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <footer class="border-t border-gray-200 py-3 px-4 sm:px-6 lg:px-8">
      <div class="max-w-7xl mx-auto flex items-center justify-between text-xs text-gray-400">
        <span>queue-ti</span>
        <span>{{ version() }}</span>
      </div>
    </footer>
  `,
})
export class FooterComponent {
  private readonly versionService = inject(VersionService);
  protected readonly version = this.versionService.version;
}
