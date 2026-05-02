import pytest
from datetime import datetime, timezone

from queueti._exceptions import AckError, NackError
from queueti._message import Message, SyncMessage


def _make_async_message(
    ack_fn=None, nack_fn=None, msg_id: str = "msg-1"
) -> Message:
    async def default_ack() -> None:
        pass

    async def default_nack(reason: str) -> None:
        pass

    return Message(
        id=msg_id,
        topic="test-topic",
        payload=b"hello",
        metadata={"k": "v"},
        created_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
        retry_count=0,
        _ack_fn=ack_fn or default_ack,
        _nack_fn=nack_fn or default_nack,
    )


def _make_sync_message(
    ack_fn=None, nack_fn=None, msg_id: str = "msg-1"
) -> SyncMessage:
    return SyncMessage(
        id=msg_id,
        topic="test-topic",
        payload=b"hello",
        metadata={"k": "v"},
        created_at=datetime(2026, 1, 1, tzinfo=timezone.utc),
        retry_count=0,
        _ack_fn=ack_fn or (lambda: None),
        _nack_fn=nack_fn or (lambda reason: None),
    )


class TestAsyncMessage:
    @pytest.mark.asyncio
    async def test_ack_calls_ack_fn(self) -> None:
        called = []

        async def ack_fn() -> None:
            called.append(True)

        msg = _make_async_message(ack_fn=ack_fn)
        await msg.ack()
        assert called == [True]

    @pytest.mark.asyncio
    async def test_nack_calls_nack_fn_with_reason(self) -> None:
        reasons = []

        async def nack_fn(reason: str) -> None:
            reasons.append(reason)

        msg = _make_async_message(nack_fn=nack_fn)
        await msg.nack("something went wrong")
        assert reasons == ["something went wrong"]

    @pytest.mark.asyncio
    async def test_ack_raises_ack_error_on_failure(self) -> None:
        async def ack_fn() -> None:
            raise RuntimeError("rpc error")

        msg = _make_async_message(ack_fn=ack_fn)
        with pytest.raises(AckError, match="msg-1"):
            await msg.ack()

    @pytest.mark.asyncio
    async def test_nack_raises_nack_error_on_failure(self) -> None:
        async def nack_fn(reason: str) -> None:
            raise RuntimeError("rpc error")

        msg = _make_async_message(nack_fn=nack_fn)
        with pytest.raises(NackError, match="msg-1"):
            await msg.nack("reason")


class TestSyncMessage:
    def test_ack_calls_ack_fn(self) -> None:
        called = []
        msg = _make_sync_message(ack_fn=lambda: called.append(True))
        msg.ack()
        assert called == [True]

    def test_nack_calls_nack_fn_with_reason(self) -> None:
        reasons: list[str] = []
        msg = _make_sync_message(nack_fn=lambda r: reasons.append(r))
        msg.nack("bad input")
        assert reasons == ["bad input"]

    def test_ack_raises_ack_error_on_failure(self) -> None:
        def bad_ack() -> None:
            raise RuntimeError("rpc error")

        msg = _make_sync_message(ack_fn=bad_ack)
        with pytest.raises(AckError, match="msg-1"):
            msg.ack()

    def test_nack_raises_nack_error_on_failure(self) -> None:
        def bad_nack(reason: str) -> None:
            raise RuntimeError("rpc error")

        msg = _make_sync_message(nack_fn=bad_nack)
        with pytest.raises(NackError, match="msg-1"):
            msg.nack("reason")
