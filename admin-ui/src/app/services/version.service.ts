import { Injectable, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';

@Injectable({ providedIn: 'root' })
export class VersionService {
  private readonly http = inject(HttpClient);

  readonly version = signal('...');

  constructor() {
    this.http.get<{ version: string }>('/api/version').subscribe({
      next: (body) => this.version.set(body.version),
      error: () => this.version.set('unknown'),
    });
  }
}
