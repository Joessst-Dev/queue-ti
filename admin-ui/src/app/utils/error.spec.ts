import { describe, it, expect } from 'vitest';
import { HttpErrorResponse } from '@angular/common/http';
import { getErrorMessage } from './error';

describe('getErrorMessage', () => {
  describe('when err is an HttpErrorResponse with a body containing an error string', () => {
    it('should return the error string from the body', () => {
      const err = new HttpErrorResponse({ error: { error: 'queue not found' }, status: 404 });
      expect(getErrorMessage(err, 'fallback')).toBe('queue not found');
    });
  });

  describe('when err is an HttpErrorResponse with a body where error is undefined', () => {
    it('should return the fallback', () => {
      const err = new HttpErrorResponse({ error: { other: 'data' }, status: 400 });
      expect(getErrorMessage(err, 'fallback')).toBe('fallback');
    });
  });

  describe('when err is an HttpErrorResponse with a null body', () => {
    it('should return the fallback', () => {
      const err = new HttpErrorResponse({ error: null, status: 500 });
      expect(getErrorMessage(err, 'fallback')).toBe('fallback');
    });
  });

  describe('when err is a plain Error', () => {
    it('should return the fallback', () => {
      expect(getErrorMessage(new Error('boom'), 'fallback')).toBe('fallback');
    });
  });

  describe('when err is a string', () => {
    it('should return the fallback', () => {
      expect(getErrorMessage('something went wrong', 'fallback')).toBe('fallback');
    });
  });

  describe('when err is null', () => {
    it('should return the fallback', () => {
      expect(getErrorMessage(null, 'fallback')).toBe('fallback');
    });
  });

  describe('when err is undefined', () => {
    it('should return the fallback', () => {
      expect(getErrorMessage(undefined, 'fallback')).toBe('fallback');
    });
  });
});
