import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { VersionService } from './version.service';

describe('VersionService', () => {
  let service: VersionService;
  let httpMock: HttpTestingController;

  const setup = () => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), VersionService],
    });
    httpMock = TestBed.inject(HttpTestingController);
    service = TestBed.inject(VersionService);
  };

  afterEach(() => httpMock.verify());

  describe('when the version endpoint responds successfully', () => {
    it('should set version to the returned value', () => {
      setup();
      const req = httpMock.expectOne('/api/version');
      req.flush({ version: '1.2.3' });
      expect(service.version()).toBe('1.2.3');
    });
  });

  describe('when the version endpoint errors', () => {
    it('should set version to "unknown"', () => {
      setup();
      const req = httpMock.expectOne('/api/version');
      req.error(new ProgressEvent('error'));
      expect(service.version()).toBe('unknown');
    });
  });

  describe('before the version endpoint responds', () => {
    it('should have the loading placeholder "..."', () => {
      setup();
      expect(service.version()).toBe('...');
      httpMock.expectOne('/api/version').flush({ version: '0.0.1' });
    });
  });
});
