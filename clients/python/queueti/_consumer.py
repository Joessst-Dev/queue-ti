from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator, Awaitable, Callable
from datetime import datetime, timezone

import grpc

from queueti._message import Message
from queueti._options import BatchOptions, ConsumerOptions
from queueti.pb import queue_pb2, queue_pb2_grpc

logger = logging.getLogger(__name__)

MessageHandler = Callable[[Message], Awaitable[None]]
BatchHandler = Callable[[list[Message]], Awaitable[None]]

_BACKOFF_START = 0.5
_BACKOFF_MAX = 30.0


def _next_backoff(current: float) -> float:
    return min(current * 2, _BACKOFF_MAX)


def _proto_ts_to_datetime(ts: object) -> datetime:
    seconds = getattr(ts, "seconds", 0) or 0
    nanos = getattr(ts, "nanos", 0) or 0
    return datetime.fromtimestamp(seconds + nanos / 1e9, tz=timezone.utc)


class AsyncConsumer:
    def __init__(
        self,
        stub: queue_pb2_grpc.QueueServiceStub,
        topic: str,
        options: ConsumerOptions | None = None,
    ) -> None:
        self._stub = stub
        self._topic = topic
        opts = options or ConsumerOptions()
        self._concurrency = opts.concurrency
        self._visibility_timeout = opts.visibility_timeout_seconds
        self._consumer_group: str = opts.consumer_group

    async def consume(self, handler: MessageHandler) -> None:
        """Stream messages from the topic, calling handler for each one.

        Runs until cancelled. Auto-acks on success, auto-nacks on handler exception.
        Reconnects with exponential backoff on stream errors.
        """
        backoff = _BACKOFF_START
        while True:
            req = queue_pb2.SubscribeRequest(topic=self._topic, consumer_group=self._consumer_group)
            if self._visibility_timeout is not None:
                req.visibility_timeout_seconds = self._visibility_timeout

            try:
                stream: AsyncIterator[queue_pb2.SubscribeResponse] = self._stub.Subscribe(req)
                clean_exit = await self._drain_stream(stream, handler)
            except asyncio.CancelledError:
                return
            except grpc.RpcError as exc:
                logger.error("queue-ti consumer: subscribe error (retrying in %.1fs): %s", backoff, exc)
                try:
                    await asyncio.sleep(backoff)
                except asyncio.CancelledError:
                    return
                backoff = _next_backoff(backoff)
                continue

            if clean_exit:
                backoff = _BACKOFF_START
            else:
                try:
                    await asyncio.sleep(backoff)
                except asyncio.CancelledError:
                    return
                backoff = _next_backoff(backoff)

    async def _drain_stream(
        self,
        stream: AsyncIterator[queue_pb2.SubscribeResponse],
        handler: MessageHandler,
    ) -> bool:
        sem = asyncio.Semaphore(self._concurrency)
        tasks: set[asyncio.Task[None]] = set()

        async def dispatch(raw: queue_pb2.SubscribeResponse) -> None:
            msg = self._build_message(raw)
            await self._dispatch(msg, handler)

        def _on_done(t: asyncio.Task[None], _sem: asyncio.Semaphore = sem) -> None:
            _sem.release()
            tasks.discard(t)

        try:
            async for raw in stream:
                await sem.acquire()
                task = asyncio.create_task(dispatch(raw))
                tasks.add(task)
                task.add_done_callback(_on_done)
        except asyncio.CancelledError:
            current = asyncio.current_task()
            if current is not None and current.cancelling() > 0:
                # External task.cancel() — abort in-flight handlers quickly.
                for t in tasks:
                    t.cancel()
            # Let tasks finish (or absorb their cancellations) before re-raising.
            await asyncio.gather(*tasks, return_exceptions=True)
            raise  # propagate so consume() exits the while loop
        except grpc.RpcError as exc:
            logger.error("queue-ti consumer: stream error (will reconnect): %s", exc)
            await asyncio.gather(*tasks, return_exceptions=True)
            return False

        await asyncio.gather(*tasks, return_exceptions=True)
        return True

    async def _dispatch(self, msg: Message, handler: MessageHandler) -> None:
        threw = False
        reason = "unknown error"
        try:
            await handler(msg)
        except Exception as exc:
            threw = True
            reason = str(exc)

        if not threw:
            try:
                await self._stub.Ack(queue_pb2.AckRequest(id=msg.id, consumer_group=self._consumer_group))
            except Exception as exc:
                logger.error("queue-ti consumer: ack failed for message %s: %s", msg.id, exc)
            return

        try:
            await self._stub.Nack(queue_pb2.NackRequest(id=msg.id, error=reason, consumer_group=self._consumer_group))
        except Exception as exc:
            logger.error("queue-ti consumer: nack failed for message %s: %s", msg.id, exc)

    async def consume_batch(self, options: BatchOptions, handler: BatchHandler) -> None:
        """Poll batches from the topic, calling handler for each batch.

        Runs until cancelled. Handler is responsible for acking/nacking messages.
        Backs off when the queue is empty.
        """
        backoff = _BACKOFF_START
        while True:
            group = options.consumer_group or self._consumer_group
            req = queue_pb2.BatchDequeueRequest(topic=self._topic, count=options.batch_size, consumer_group=group)
            if options.visibility_timeout_seconds is not None:
                req.visibility_timeout_seconds = options.visibility_timeout_seconds

            try:
                resp: queue_pb2.BatchDequeueResponse = await self._stub.BatchDequeue(req)
            except asyncio.CancelledError:
                return
            except grpc.RpcError as exc:
                logger.error("queue-ti consumer: batchDequeue error (retrying in %.1fs): %s", backoff, exc)
                try:
                    await asyncio.sleep(backoff)
                except asyncio.CancelledError:
                    return
                backoff = _next_backoff(backoff)
                continue

            if not resp.messages:
                try:
                    await asyncio.sleep(backoff)
                except asyncio.CancelledError:
                    return
                backoff = _next_backoff(backoff)
                continue

            backoff = _BACKOFF_START
            batch = [self._build_message(raw, group) for raw in resp.messages]
            try:
                await handler(batch)
            except asyncio.CancelledError:
                return
            except Exception as exc:
                logger.error("queue-ti consumer: batch handler error: %s", exc)

    def _build_message(self, raw: queue_pb2.SubscribeResponse | queue_pb2.DequeueResponse, consumer_group: str = "") -> Message:
        group = consumer_group or self._consumer_group

        async def ack_fn() -> None:
            await self._stub.Ack(queue_pb2.AckRequest(id=raw.id, consumer_group=group))

        async def nack_fn(reason: str) -> None:
            await self._stub.Nack(queue_pb2.NackRequest(id=raw.id, error=reason, consumer_group=group))

        return Message(
            id=raw.id,
            topic=raw.topic,
            payload=bytes(raw.payload),
            metadata=dict(raw.metadata),
            created_at=_proto_ts_to_datetime(raw.created_at),
            retry_count=raw.retry_count,
            _ack_fn=ack_fn,
            _nack_fn=nack_fn,
        )
