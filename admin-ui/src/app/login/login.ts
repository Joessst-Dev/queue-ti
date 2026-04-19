import { Component, inject, computed, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { FormField, form, schema, required } from '@angular/forms/signals';
import { Router } from '@angular/router';
import { Subject, switchMap, map, tap, startWith } from 'rxjs';
import { AuthService } from '../services/auth.service';

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
  imports: [FormField],
  template: `
    <div class="min-h-screen flex items-center justify-center bg-gray-50">
      <div class="w-full max-w-sm">
        <div class="bg-white shadow rounded-lg p-8">
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

          <form (ngSubmit)="onLogin()" class="space-y-4">
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
              class="w-full py-2 px-4 bg-indigo-600 text-white font-medium rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {{ loading() ? 'Signing in...' : 'Sign in' }}
            </button>
          </form>
        </div>
      </div>
    </div>
  `,
})
export class Login {
  private auth = inject(AuthService);
  private router = inject(Router);

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
            if (success) this.router.navigate(['/messages']);
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

  onLogin() {
    this.loginTrigger$.next({
      username: this.loginForm.username().value(),
      password: this.loginForm.password().value(),
    });
  }
}
