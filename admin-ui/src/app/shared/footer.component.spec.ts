import { TestBed, ComponentFixture } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { FooterComponent } from './footer.component';
import { VersionService } from '../services/version.service';

describe('FooterComponent', () => {
  let fixture: ComponentFixture<FooterComponent>;
  let httpMock: HttpTestingController;

  const setup = () => {
    TestBed.configureTestingModule({
      imports: [FooterComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), VersionService],
    });
    httpMock = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(FooterComponent);
  };

  afterEach(() => httpMock.verify());

  describe('when version is loading', () => {
    it('should render the loading placeholder', () => {
      setup();
      fixture.detectChanges();
      const spans = fixture.nativeElement.querySelectorAll('span');
      expect(spans[0].textContent.trim()).toBe('queue-ti');
      expect(spans[1].textContent.trim()).toBe('...');
      httpMock.expectOne('/api/version').flush({ version: '1.0.0' });
    });
  });

  describe('when version resolves', () => {
    it('should display the resolved version string', async () => {
      setup();
      fixture.detectChanges();
      httpMock.expectOne('/api/version').flush({ version: '2.5.0' });
      fixture.detectChanges();
      const spans = fixture.nativeElement.querySelectorAll('span');
      expect(spans[0].textContent.trim()).toBe('queue-ti');
      expect(spans[1].textContent.trim()).toBe('2.5.0');
    });
  });

  describe('when version endpoint errors', () => {
    it('should display "unknown"', async () => {
      setup();
      fixture.detectChanges();
      httpMock.expectOne('/api/version').error(new ProgressEvent('error'));
      fixture.detectChanges();
      const spans = fixture.nativeElement.querySelectorAll('span');
      expect(spans[0].textContent.trim()).toBe('queue-ti');
      expect(spans[1].textContent.trim()).toBe('unknown');
    });
  });

  describe('template structure', () => {
    it('should render a <footer> element', () => {
      setup();
      fixture.detectChanges();
      httpMock.expectOne('/api/version').flush({ version: '1.0.0' });
      expect(fixture.nativeElement.querySelector('footer')).not.toBeNull();
    });
  });
});
