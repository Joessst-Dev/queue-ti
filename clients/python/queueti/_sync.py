"""Synchronous wrapper around the async client.

Runs a dedicated event loop on a background thread so sync callers never
need to touch asyncio directly.
"""

from __future__ import annotations

import asyncio
import concurrent.futures
import threading
from collections.abc import Callable
from typing import TypeVar

from queueti._client import AsyncClient, connect
from queueti._consumer import AsyncConsumer, BatchHandler, MessageHandler
from queueti._message import Message, SyncMessage
from queueti._options import BatchOptions, ConnectOptions, ConsumerOptions, PublishOptions
from queueti._producer import AsyncProducer

T = TypeVar("T")


class _LoopThread(threading.Thread):
    """A daemon thread that owns a dedicated asyncio event loop."""

    def __init__(self) -> None:
        super().__init__(daemon=True)
        self._loop = asyncio.new_event_loop()
        self._ready = threading.Event()

    def run(self) -> None:
        asyncio.set_event_loop(self._loop)
        self._ready.set()
        self._loop.run_forever()

    def submit(self, coro: object) -> concurrent.futures.Future[T]:  # type: ignore[type-arg]
        return asyncio.run_coroutine_threadsafe(coro, self._loop)  # type: ignore[arg-type]

    def run_sync(self, coro: object) -> T:  # type: ignore[type-var]
        return self.submit(coro).result()

    def stop(self) -> None:
        self._loop.call_soon_threadsafe(self._loop.stop)
        self.join()


class Producer:
    """Synchronous producer. Obtain via :meth:`Client.producer`."""

    def __init__(self, async_producer: AsyncProducer, loop: _LoopThread) -> None:
        self._async = async_producer
        self._loop = loop

    def publish(
        self,
        topic: str,
        payload: bytes,
        options: PublishOptions | None = None,
    ) -> str:
        """Publish a message and return its assigned ID."""
        return self._loop.run_sync(self._async.publish(topic, payload, options))


class Consumer:
    """Synchronous consumer. Obtain via :meth:`Client.consumer`."""

    def __init__(self, async_consumer: AsyncConsumer, loop: _LoopThread) -> None:
        self._async = async_consumer
        self._loop = loop

    def consume(self, handler: Callable[[SyncMessage], None]) -> None:
        """Block and process messages until the thread is interrupted.

        The handler receives a :class:`SyncMessage` with synchronous ``ack()``
        and ``nack()`` methods. Auto-acks on success, auto-nacks on exception.
        """

        async def async_handler(msg: Message) -> None:
            sync_msg = self._to_sync_message(msg)
            await asyncio.get_event_loop().run_in_executor(None, handler, sync_msg)

        self._loop.run_sync(self._async.consume(async_handler))

    def consume_batch(
        self,
        options: BatchOptions,
        handler: Callable[[list[SyncMessage]], None],
    ) -> None:
        """Block and process batches until the thread is interrupted.

        The handler receives a list of :class:`SyncMessage` objects.
        Handler is responsible for calling ``ack()`` / ``nack()`` on each.
        """

        async def async_handler(msgs: list[Message]) -> None:
            sync_msgs = [self._to_sync_message(m) for m in msgs]
            await asyncio.get_event_loop().run_in_executor(None, handler, sync_msgs)

        self._loop.run_sync(self._async.consume_batch(options, async_handler))

    def _to_sync_message(self, msg: Message) -> SyncMessage:
        def ack_fn() -> None:
            self._loop.run_sync(msg._ack_fn())

        def nack_fn(reason: str) -> None:
            self._loop.run_sync(msg._nack_fn(reason))

        return SyncMessage(
            id=msg.id,
            topic=msg.topic,
            payload=msg.payload,
            metadata=msg.metadata,
            created_at=msg.created_at,
            retry_count=msg.retry_count,
            _ack_fn=ack_fn,
            _nack_fn=nack_fn,
        )


class Client:
    """Synchronous client. Obtain via :func:`connect_sync`."""

    def __init__(self, async_client: AsyncClient, loop: _LoopThread) -> None:
        self._async = async_client
        self._loop = loop

    def producer(self) -> Producer:
        return Producer(self._async.producer(), self._loop)

    def consumer(self, topic: str, options: ConsumerOptions | None = None) -> Consumer:
        return Consumer(self._async.consumer(topic, options), self._loop)

    def set_token(self, token: str) -> None:
        self._async.set_token(token)

    def close(self) -> None:
        self._loop.run_sync(self._async.close())
        self._loop.stop()


def connect_sync(address: str, options: ConnectOptions | None = None) -> Client:
    """Connect to a queue-ti server and return a synchronous :class:`Client`.

    :param address: Server address, e.g. ``"localhost:50051"``.
    :param options: Optional :class:`ConnectOptions` for auth and TLS.
    """
    loop = _LoopThread()
    loop.start()
    loop._ready.wait()
    async_client = loop.run_sync(connect(address, options))
    return Client(async_client, loop)
