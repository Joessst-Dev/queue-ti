import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { map } from 'rxjs';
import { AuthService } from '../services/auth.service';

export const authGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);

  return auth.checkAuthStatus().pipe(
    map((required) => {
      if (!required || auth.isAuthenticated()) return true;
      return router.createUrlTree(['/login']);
    }),
  );
};

export const loginGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);

  return auth.checkAuthStatus().pipe(
    map((required) => {
      if (!required || auth.isAuthenticated()) {
        return router.createUrlTree(['/messages']);
      }
      return true;
    }),
  );
};
