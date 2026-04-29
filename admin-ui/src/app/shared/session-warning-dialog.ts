import { Component, ChangeDetectionStrategy, inject } from '@angular/core';
import { DialogRef } from '@angular/cdk/dialog';
import { Router } from '@angular/router';
import { AuthService } from '../services/auth.service';
import { SessionService } from '../services/session.service';

@Component({
  selector: 'app-session-warning-dialog',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="bg-white rounded-lg shadow-xl p-6 w-full max-w-sm mx-auto">
      <div class="flex justify-center mb-4">
        <svg
          class="w-10 h-10 text-amber-500"
          fill="none"
          viewBox="0 0 24 24"
          stroke-width="1.5"
          stroke="currentColor"
          aria-hidden="true"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z"
          />
        </svg>
      </div>
      <h2 class="text-lg font-semibold text-gray-900 text-center mb-2">
        Still there?
      </h2>
      <p class="text-sm text-gray-600 text-center mb-6">
        Your session will expire in
        <span class="font-semibold text-gray-900">{{ sessionService.secondsRemaining() }}</span>
        seconds. Would you like to stay logged in?
      </p>
      <div class="flex flex-col gap-2">
        <button
          type="button"
          (click)="extend()"
          class="w-full py-2 px-4 bg-indigo-600 text-white font-medium rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 cursor-pointer"
        >
          Stay logged in
        </button>
        <button
          type="button"
          (click)="logout()"
          class="w-full py-2 px-4 bg-white text-gray-700 font-medium rounded-md border border-gray-300 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 cursor-pointer"
        >
          Log out
        </button>
      </div>
    </div>
  `,
})
export class SessionWarningDialog {
  protected readonly sessionService = inject(SessionService);
  private readonly dialogRef = inject(DialogRef);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);

  extend(): void {
    this.sessionService.extendSession();
    this.dialogRef.close();
  }

  logout(): void {
    this.auth.logout();
    this.router.navigate(['/login']);
    this.dialogRef.close();
  }
}
