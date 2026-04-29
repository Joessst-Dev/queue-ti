import { Component } from '@angular/core';
import { RouterModule } from '@angular/router';
import { ActivityTrackerDirective } from './shared/activity-tracker.directive';
import { SessionManagerComponent } from './shared/session-manager.component';

@Component({
  imports: [RouterModule, SessionManagerComponent],
  hostDirectives: [ActivityTrackerDirective],
  selector: 'app-root',
  template: `
    <router-outlet />
    <app-session-manager />
  `,
})
export class App {}
