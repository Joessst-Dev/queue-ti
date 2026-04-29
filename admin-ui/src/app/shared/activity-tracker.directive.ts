import { Directive, DestroyRef, inject } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { DOCUMENT } from '@angular/common';
import { fromEvent, merge } from 'rxjs';
import { debounceTime } from 'rxjs/operators';
import { SessionService } from '../services/session.service';

@Directive({ selector: '[appActivityTracker]' })
export class ActivityTrackerDirective {
  private readonly sessionService = inject(SessionService);
  private readonly document = inject(DOCUMENT);
  private readonly destroyRef = inject(DestroyRef);

  constructor() {
    merge(
      fromEvent(this.document, 'mousemove'),
      fromEvent(this.document, 'keydown'),
      fromEvent(this.document, 'pointerdown'),
      fromEvent(this.document, 'scroll'),
      fromEvent(this.document, 'touchstart'),
    )
      .pipe(debounceTime(2000), takeUntilDestroyed(this.destroyRef))
      .subscribe(() => this.sessionService.recordActivity());
  }
}
