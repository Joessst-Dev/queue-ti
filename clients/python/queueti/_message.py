from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Awaitable, Callable

from queueti._exceptions import AckError, NackError


@dataclass
class Message:
    """A message received from the queue. Used with AsyncConsumer.

    Call ``await msg.ack()`` when processing succeeds, or ``await msg.nack(reason)``
    to return the message to the queue.  The consumer calls these automatically
    when you use :meth:`AsyncConsumer.consume` — only call them directly when
    using :meth:`AsyncConsumer.consume_batch`.
    """

    id: str
    topic: str
    payload: bytes
    metadata: dict[str, str]
    created_at: datetime
    retry_count: int
    _ack_fn: Callable[[], Awaitable[None]]
    _nack_fn: Callable[[str], Awaitable[None]]

    async def ack(self) -> None:
        try:
            await self._ack_fn()
        except Exception as exc:
            raise AckError(f"ack failed for message {self.id}: {exc}") from exc

    async def nack(self, reason: str = "") -> None:
        try:
            await self._nack_fn(reason)
        except Exception as exc:
            raise NackError(f"nack failed for message {self.id}: {exc}") from exc


@dataclass
class SyncMessage:
    """A message received from the queue. Used with Consumer (sync).

    Call ``msg.ack()`` when processing succeeds, or ``msg.nack(reason)``
    to return the message to the queue.  The consumer calls these automatically
    when you use :meth:`Consumer.consume` — only call them directly when
    using :meth:`Consumer.consume_batch`.
    """

    id: str
    topic: str
    payload: bytes
    metadata: dict[str, str]
    created_at: datetime
    retry_count: int
    _ack_fn: Callable[[], None]
    _nack_fn: Callable[[str], None]

    def ack(self) -> None:
        try:
            self._ack_fn()
        except Exception as exc:
            raise AckError(f"ack failed for message {self.id}: {exc}") from exc

    def nack(self, reason: str = "") -> None:
        try:
            self._nack_fn(reason)
        except Exception as exc:
            raise NackError(f"nack failed for message {self.id}: {exc}") from exc
