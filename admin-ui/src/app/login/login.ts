import { Component, inject, computed, signal, ChangeDetectionStrategy } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { FormField, form, schema, required } from '@angular/forms/signals';
import { SpinnerComponent } from '../shared/spinner.component';
import { Router } from '@angular/router';
import { Subject, switchMap, map, tap, startWith } from 'rxjs';
import { AuthService } from '../services/auth.service';
import { SessionService } from '../services/session.service';

interface LoginState {
  loading: boolean;
  error: string;
}

interface LoginModel {
  username: string;
  password: string;
}

@Component({
  selector: 'app-login',
  imports: [FormField, SpinnerComponent],
  template: `
    <div class="min-h-screen flex items-center justify-center bg-gray-50">
      <div class="w-full max-w-sm">
        <div class="bg-white shadow rounded-lg p-8">
          <div class="flex justify-center mb-4">
            <svg
              class="w-10 h-10 text-indigo-600"
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
          </div>
          <h1 class="text-2xl font-bold text-gray-900 mb-6 text-center">
            Queue-ti Admin
          </h1>

          @if (error()) {
            <div
              class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm"
            >
              {{ error() }}
            </div>
          }

          <form (submit)="onLogin($event)" class="space-y-4">
            <div>
              <label
                for="username"
                class="block text-sm font-medium text-gray-700 mb-1"
                >Username</label
              >
              <input
                id="username"
                type="text"
                [formField]="loginForm.username"
                class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
              />
            </div>
            <div>
              <label
                for="password"
                class="block text-sm font-medium text-gray-700 mb-1"
                >Password</label
              >
              <input
                id="password"
                type="password"
                [formField]="loginForm.password"
                class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
              />
            </div>
            <button
              type="submit"
              [disabled]="loading()"
              class="flex items-center justify-center gap-2 w-full py-2 px-4 bg-indigo-600 text-white font-medium rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
            >
              @if (loading()) {
                <app-spinner />
                Signing in...
              } @else {
                <svg
                  class="inline w-4 h-4 mr-1"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M15.75 9V5.25A2.25 2.25 0 0 0 13.5 3h-6a2.25 2.25 0 0 0-2.25 2.25v13.5A2.25 2.25 0 0 0 7.5 21h6a2.25 2.25 0 0 0 2.25-2.25V15m3 0 3-3m0 0-3-3m3 3H9"
                  />
                </svg>
                Sign in
              }
            </button>
          </form>
        </div>
      </div>
    </div>
  `,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Login {
  private auth = inject(AuthService);
  private router = inject(Router);
  private session = inject(SessionService);

  private loginModel = signal<LoginModel>({ username: '', password: '' });

  loginForm = form(
    this.loginModel,
    schema<LoginModel>((root) => {
      required(root.username);
      required(root.password);
    }),
  );

  private loginTrigger$ = new Subject<{ username: string; password: string }>();

  private loginState = toSignal(
    this.loginTrigger$.pipe(
      switchMap(({ username, password }) =>
        this.auth.login(username, password).pipe(
          tap((success) => {
            if (success) {
              this.session.recordActivity();
              this.router.navigate(['/messages']);
            }
          }),
          map((success) =>
            success
              ? ({ loading: false, error: '' } as LoginState)
              : ({
                  loading: false,
                  error: 'Invalid credentials',
                } as LoginState),
          ),
          startWith({ loading: true, error: '' } as LoginState),
        ),
      ),
    ),
    { initialValue: { loading: false, error: '' } },
  );

  error = computed(() => this.loginState().error);
  loading = computed(() => this.loginState().loading);

  onLogin(event: Event): void {
    event.preventDefault();
    this.loginTrigger$.next({
      username: this.loginForm.username().value(),
      password: this.loginForm.password().value(),
    });
  }
}
