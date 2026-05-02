from __future__ import annotations

import asyncio
import logging

import grpc
import grpc.aio

from queueti._consumer import AsyncConsumer
from queueti._options import ConnectOptions, ConsumerOptions, TokenRefresher
from queueti._producer import AsyncProducer
from queueti._token_store import TokenStore, seconds_until_expiry
from queueti.pb import queue_pb2_grpc

logger = logging.getLogger(__name__)

_RETRY_BACKOFF_START = 5.0
_RETRY_BACKOFF_MAX = 60.0


class AsyncClient:
    def __init__(
        self,
        channel: grpc.aio.Channel,
        stub: queue_pb2_grpc.QueueServiceStub,
        store: TokenStore | None,
        options: ConnectOptions | None,
    ) -> None:
        self._channel = channel
        self._stub = stub
        self._store = store
        self._refresh_task: asyncio.Task[None] | None = None

        if options and options.token_refresher and store:
            self._refresh_task = asyncio.create_task(
                self._run_refresher(options.token_refresher, store)
            )

    def producer(self) -> AsyncProducer:
        return AsyncProducer(self._stub)

    def consumer(self, topic: str, options: ConsumerOptions | None = None) -> AsyncConsumer:
        return AsyncConsumer(self._stub, topic, options)

    def set_token(self, token: str) -> None:
        if self._store is None:
            raise RuntimeError("set_token: client was not created with a bearer token")
        self._store.set(token)

    async def close(self) -> None:
        if self._refresh_task:
            self._refresh_task.cancel()
            try:
                await self._refresh_task
            except asyncio.CancelledError:
                pass
        await self._channel.close()

    async def _run_refresher(self, refresher: TokenRefresher, store: TokenStore) -> None:
        retry_backoff = _RETRY_BACKOFF_START

        while True:
            try:
                secs = seconds_until_expiry(store.get())
                sleep_secs = max(secs, 0.0)
                if sleep_secs > 0:
                    retry_backoff = _RETRY_BACKOFF_START
            except ValueError:
                logger.error("queue-ti: token refresher: cannot parse token expiry")
                sleep_secs = retry_backoff

            if sleep_secs > 0:
                try:
                    await asyncio.sleep(sleep_secs)
                except asyncio.CancelledError:
                    return

            try:
                new_token: str = await refresher()
            except asyncio.CancelledError:
                return
            except Exception as exc:
                logger.error(
                    "queue-ti: token refresher: refresh failed (retrying in %.1fs): %s",
                    retry_backoff,
                    exc,
                )
                try:
                    await asyncio.sleep(retry_backoff)
                except asyncio.CancelledError:
                    return
                retry_backoff = min(retry_backoff * 2, _RETRY_BACKOFF_MAX)
                continue

            store.set(new_token)
            retry_backoff = _RETRY_BACKOFF_START


async def connect(address: str, options: ConnectOptions | None = None) -> AsyncClient:
    """Connect to a queue-ti server and return an :class:`AsyncClient`.

    :param address: Server address, e.g. ``"localhost:50051"``.
    :param options: Optional :class:`ConnectOptions` for auth and TLS.
    """
    store: TokenStore | None = None

    if options and options.token:
        store = TokenStore(options.token)

    if options and options.insecure:
        channel_creds = None
    else:
        channel_creds = grpc.ssl_channel_credentials()

    if store is not None:
        token_creds = grpc.metadata_call_credentials(
            _BearerPlugin(store)
        )
        if channel_creds is None:
            # insecure + token: combine with local channel credentials
            composite = grpc.composite_channel_credentials(
                grpc.local_channel_credentials(), token_creds
            )
        else:
            composite = grpc.composite_channel_credentials(channel_creds, token_creds)
        channel = grpc.aio.secure_channel(address, composite)
    elif options and options.insecure:
        channel = grpc.aio.insecure_channel(address)
    else:
        channel = grpc.aio.secure_channel(address, channel_creds)

    stub = queue_pb2_grpc.QueueServiceStub(channel)
    return AsyncClient(channel, stub, store, options)


class _BearerPlugin(grpc.AuthMetadataPlugin):
    def __init__(self, store: TokenStore) -> None:
        self._store = store

    def __call__(
        self,
        context: grpc.AuthMetadataContext,
        callback: grpc.AuthMetadataPluginCallback,
    ) -> None:
        callback((("authorization", f"Bearer {self._store.get()}"),), None)
