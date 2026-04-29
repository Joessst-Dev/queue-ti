import { HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { tap } from 'rxjs';
import { AuthService } from '../services/auth.service';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(AuthService);
  const router = inject(Router);
  const header = auth.getAuthHeader();

  if (header && !req.headers.has('Authorization')) {
    req = req.clone({
      setHeaders: { Authorization: header },
    });
  }

  return next(req).pipe(
    tap({
      error: (err) => {
        if (err.status === 401 && auth.isAuthenticated()) {
          auth.logout();
          if (!req.url.includes('/auth/refresh')) {
            router.navigate(['/login']);
          }
        }
      },
    }),
  );
};
