import { Component, ChangeDetectionStrategy, effect, inject } from '@angular/core';
import { Dialog, DialogRef } from '@angular/cdk/dialog';
import { SessionService } from '../services/session.service';
import { SessionWarningDialog } from './session-warning-dialog';

@Component({
  selector: 'app-session-manager',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: '',
})
export class SessionManagerComponent {
  private readonly sessionService = inject(SessionService);
  private readonly dialog = inject(Dialog);

  private dialogRef: DialogRef<unknown, SessionWarningDialog> | null = null;

  constructor() {
    effect(() => {
      const show = this.sessionService.showWarning();

      if (show && this.dialogRef === null) {
        this.dialogRef = this.dialog.open(SessionWarningDialog, {
          disableClose: true,
        });
        this.dialogRef.closed.subscribe(() => {
          this.dialogRef = null;
        });
      } else if (!show && this.dialogRef !== null) {
        this.dialogRef.close();
      }
    });
  }
}
