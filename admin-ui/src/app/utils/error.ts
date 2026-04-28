import { HttpErrorResponse } from '@angular/common/http';

export function getErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof HttpErrorResponse) {
    const body = err.error as { error?: string } | null;
    return body?.error ?? fallback;
  }
  return fallback;
}
