import { TestBed } from '@angular/core/testing';
import {
  HttpTestingController,
  provideHttpClientTesting,
} from '@angular/common/http/testing';
import { provideHttpClient } from '@angular/common/http';
import { QueueService, QueueMessage, EnqueueRequest } from './queue.service';

const makeMessage = (overrides: Partial<QueueMessage> = {}): QueueMessage => ({
  id: 'msg-1',
  topic: 'default',
  payload: '{}',
  metadata: {},
  status: 'pending',
  created_at: '2024-01-01T00:00:00Z',
  retry_count: 0,
  max_retries: 3,
  last_error: '',
  expires_at: null,
  original_topic: null,
  dlq_moved_at: null,
  ...overrides,
});

describe('QueueService', () => {
  let service: QueueService;
  let httpController: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(QueueService);
    httpController = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpController.verify();
  });

  describe('listMessages()', () => {
    describe('when called without a topic', () => {
      it('should make GET /api/messages with no params', () => {
        service.listMessages().subscribe();

        const req = httpController.expectOne('/api/messages');
        expect(req.request.method).toBe('GET');
        expect(req.request.params.keys()).toHaveLength(0);
        req.flush([]);
      });

      it('should return the list of messages from the server', () => {
        const messages = [makeMessage({ id: 'msg-1' }), makeMessage({ id: 'msg-2' })];
        let result: QueueMessage[] | undefined;

        service.listMessages().subscribe((v) => (result = v));
        httpController.expectOne('/api/messages').flush(messages);

        expect(result).toEqual(messages);
      });
    });

    describe('when called with a topic', () => {
      it('should add ?topic=<topic> to the request', () => {
        service.listMessages('orders').subscribe();

        const req = httpController.expectOne(
          (r) => r.url === '/api/messages' && r.params.get('topic') === 'orders',
        );
        expect(req.request.method).toBe('GET');
        req.flush([]);
      });

      it('should not include topic param when topic is an empty string', () => {
        service.listMessages('').subscribe();

        const req = httpController.expectOne('/api/messages');
        expect(req.request.params.has('topic')).toBe(false);
        req.flush([]);
      });
    });
  });

  describe('enqueueMessage()', () => {
    const enqueueReq: EnqueueRequest = {
      topic: 'orders',
      payload: '{"orderId": 42}',
      metadata: { source: 'test' },
    };

    it('should make POST /api/messages with the request body', () => {
      service.enqueueMessage(enqueueReq).subscribe();

      const req = httpController.expectOne('/api/messages');
      expect(req.request.method).toBe('POST');
      expect(req.request.body).toEqual(enqueueReq);
      req.flush({ id: 'new-msg-id' });
    });

    it('should return the enqueued message id', () => {
      let result: { id: string } | undefined;
      service.enqueueMessage(enqueueReq).subscribe((v) => (result = v));
      httpController.expectOne('/api/messages').flush({ id: 'new-msg-id' });

      expect(result).toEqual({ id: 'new-msg-id' });
    });
  });

  describe('nackMessage()', () => {
    describe('when called with an id and an error string', () => {
      it('should make POST /api/messages/:id/nack with the error body', () => {
        service.nackMessage('msg-42', 'something went wrong').subscribe();

        const req = httpController.expectOne('/api/messages/msg-42/nack');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ error: 'something went wrong' });
        req.flush(null);
      });
    });

    describe('when called without an error string', () => {
      it('should send an empty error string in the body', () => {
        service.nackMessage('msg-99').subscribe();

        const req = httpController.expectOne('/api/messages/msg-99/nack');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ error: '' });
        req.flush(null);
      });
    });
  });

  describe('requeueMessage()', () => {
    describe('when called with an id', () => {
      it('should make POST /api/messages/:id/requeue with an empty body', () => {
        service.requeueMessage('msg-dlq-1').subscribe();

        const req = httpController.expectOne('/api/messages/msg-dlq-1/requeue');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({});
        req.flush(null);
      });
    });
  });
});
