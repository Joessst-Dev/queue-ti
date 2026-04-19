import { Route } from '@angular/router';
import { authGuard, loginGuard } from './guards/auth.guard';

export const appRoutes: Route[] = [
  {
    path: 'login',
    loadComponent: () => import('./login/login').then((m) => m.Login),
    canActivate: [loginGuard],
  },
  {
    path: 'messages',
    loadComponent: () => import('./messages/messages').then((m) => m.Messages),
    canActivate: [authGuard],
  },
  { path: '', redirectTo: 'messages', pathMatch: 'full' },
  { path: '**', redirectTo: 'messages' },
];
