import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection, Component, Input } from '@angular/core';
import { provideRouter } from '@angular/router';
import { AdminComponent } from './admin';
import { AuthService } from '../services/auth.service';
import { MessagesHeader } from '../messages/messages-header';
import { UsersSection } from './users-section';
import { MaintenanceSection } from './maintenance-section';

@Component({ selector: 'app-messages-header', template: '<div>header</div>', standalone: true })
class StubMessagesHeader {
  @Input() isAuthenticated = false;
}

@Component({ selector: 'app-users-section', template: '<div>users</div>', standalone: true })
class StubUsersSection {}

@Component({ selector: 'app-maintenance-section', template: '<div>maintenance</div>', standalone: true })
class StubMaintenanceSection {}

const makeAuthService = () =>
  ({
    isAuthenticated: () => true,
    isAdmin: () => true,
    logout: vi.fn(),
  }) as unknown as AuthService;

const setup = async () => {
  await TestBed.configureTestingModule({
    imports: [AdminComponent],
    providers: [
      provideZonelessChangeDetection(),
      provideRouter([]),
      { provide: AuthService, useValue: makeAuthService() },
    ],
  })
    .overrideComponent(AdminComponent, {
      remove: { imports: [MessagesHeader, UsersSection, MaintenanceSection] },
      add: { imports: [StubMessagesHeader, StubUsersSection, StubMaintenanceSection] },
    })
    .compileComponents();

  const fixture = TestBed.createComponent(AdminComponent);
  fixture.detectChanges();
  await fixture.whenStable();
  return { fixture };
};

describe('AdminComponent', () => {
  describe('when rendered', () => {
    it('should render the header', async () => {
      const { fixture } = await setup();
      expect(fixture.nativeElement.querySelector('app-messages-header')).not.toBeNull();
    });

    it('should render users section', async () => {
      const { fixture } = await setup();
      expect(fixture.nativeElement.querySelector('app-users-section')).not.toBeNull();
    });

    it('should render maintenance section', async () => {
      const { fixture } = await setup();
      expect(fixture.nativeElement.querySelector('app-maintenance-section')).not.toBeNull();
    });
  });
});
