import { Component, inject, ChangeDetectionStrategy } from '@angular/core';
import { Router } from '@angular/router';
import { AuthService } from '../services/auth.service';
import { MessagesHeader } from '../messages/messages-header';
import { UsersSection } from './users-section';
import { MaintenanceSection } from './maintenance-section';

@Component({
  selector: 'app-admin',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MessagesHeader, UsersSection, MaintenanceSection],
  template: `
    <div class="min-h-screen bg-gray-50">
      <app-messages-header [isAuthenticated]="auth.isAuthenticated()" (signOut)="onLogout()" />

      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-6">
        <div><app-users-section /></div>
        <div><app-maintenance-section /></div>
      </div>
    </div>
  `,
})
export class AdminComponent {
  protected readonly auth = inject(AuthService);
  private readonly router = inject(Router);

  onLogout(): void {
    this.auth.logout();
    this.router.navigate(['/login']);
  }
}
