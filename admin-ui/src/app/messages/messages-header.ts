import { Component, input, output, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { AuthService } from '../services/auth.service';
import { inject } from '@angular/core';

@Component({
  selector: 'app-messages-header',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, RouterLinkActive],
  template: `
    <header class="bg-white shadow-sm">
      <div
        class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4 flex items-center justify-between"
      >
        <div class="flex items-center gap-6">
          <div class="flex items-center gap-2">
            <svg
              class="w-6 h-6 text-indigo-600"
              fill="none"
              viewBox="0 0 24 24"
              stroke-width="1.5"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M3.75 12h16.5m-16.5 3.75h16.5M3.75 19.5h16.5M5.625 4.5h12.75a1.875 1.875 0 0 1 0 3.75H5.625a1.875 1.875 0 0 1 0-3.75Z"
              />
            </svg>
            <h1 class="text-xl font-bold text-gray-900">QueueTI Admin</h1>
          </div>
          <nav class="flex items-center gap-4" aria-label="Site navigation">
            <a
              routerLink="/messages"
              routerLinkActive="text-indigo-600 font-semibold"
              [routerLinkActiveOptions]="{ exact: false }"
              class="text-sm text-gray-600 hover:text-gray-900"
            >
              Messages
            </a>
            @if (auth.isAdmin()) {
              <a
                routerLink="/admin"
                routerLinkActive="text-indigo-600 font-semibold"
                [routerLinkActiveOptions]="{ exact: false }"
                class="text-sm text-gray-600 hover:text-gray-900"
              >
                Admin
              </a>
            }
          </nav>
        </div>
        @if (isAuthenticated()) {
          <button
            (click)="signOut.emit()"
            class="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 cursor-pointer"
          >
            <svg
              class="w-4 h-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke-width="1.5"
              stroke="currentColor"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M8.25 9V5.25A2.25 2.25 0 0 1 10.5 3h6a2.25 2.25 0 0 1 2.25 2.25v13.5A2.25 2.25 0 0 1 16.5 21h-6a2.25 2.25 0 0 1-2.25-2.25V15m-3 0-3-3m0 0 3-3m-3 3H15"
              />
            </svg>
            Sign out
          </button>
        }
      </div>
    </header>
  `,
})
export class MessagesHeader {
  readonly isAuthenticated = input.required<boolean>();
  readonly signOut = output<void>();
  protected readonly auth = inject(AuthService);
}
