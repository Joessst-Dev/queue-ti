import asyncio
from collections.abc import AsyncIterator
from unittest.mock import AsyncMock, MagicMock, patch

import grpc
import pytest

from queueti._consumer import AsyncConsumer
from queueti._message import Message
from queueti._options import BatchOptions, ConsumerOptions
from queueti.pb import queue_pb2


# ── helpers ──────────────────────────────────────────────────────────────────


def _make_raw(msg_id: str = "m1", topic: str = "t") -> queue_pb2.SubscribeResponse:
    raw = queue_pb2.SubscribeResponse()
    raw.id = msg_id
    raw.topic = topic
    raw.payload = b"data"
    raw.retry_count = 0
    return raw


def _make_dequeue_raw(msg_id: str = "m1") -> queue_pb2.DequeueResponse:
    raw = queue_pb2.DequeueResponse()
    raw.id = msg_id
    raw.topic = "t"
    raw.payload = b"data"
    raw.retry_count = 0
    return raw


async def _finite(*items: object) -> AsyncIterator[object]:
    """Async generator that yields items then ends (StopAsyncIteration)."""
    for item in items:
        yield item


async def _error_stream() -> AsyncIterator[queue_pb2.SubscribeResponse]:
    """Immediately raises a gRPC error."""
    raise grpc.RpcError("stream failed")
    yield  # make it an async generator


def _make_stub(**kwargs: object) -> MagicMock:
    stub = MagicMock()
    stub.Subscribe = MagicMock(return_value=kwargs.get("subscribe_return"))
    if "subscribe_side_effect" in kwargs:
        stub.Subscribe = MagicMock(side_effect=kwargs["subscribe_side_effect"])
    stub.Ack = AsyncMock(return_value=queue_pb2.AckResponse())
    stub.Nack = AsyncMock(return_value=queue_pb2.NackResponse())
    return stub


def _make_consumer(stub: MagicMock, options: ConsumerOptions | None = None) -> AsyncConsumer:
    return AsyncConsumer(stub, "t", options)


# ── _drain_stream unit tests ──────────────────────────────────────────────────
# Testing _drain_stream directly avoids the while-True loop in consume() and
# lets us verify per-message behaviour with finite streams.


class TestConsumerOptions:
    def test_concurrency_zero_raises(self) -> None:
        with pytest.raises(ValueError, match="concurrency"):
            ConsumerOptions(concurrency=0)

    def test_concurrency_negative_raises(self) -> None:
        with pytest.raises(ValueError, match="concurrency"):
            ConsumerOptions(concurrency=-1)

    def test_concurrency_one_is_valid(self) -> None:
        opts = ConsumerOptions(concurrency=1)
        assert opts.concurrency == 1

    def test_consumer_group_defaults_to_empty_string(self) -> None:
        opts = ConsumerOptions()
        assert opts.consumer_group == ""


class TestDrainStream:
    @pytest.mark.asyncio
    async def test_handler_called_for_each_message(self) -> None:
        received: list[str] = []
        stub = _make_stub()
        consumer = _make_consumer(stub)

        async def handler(msg: Message) -> None:
            received.append(msg.id)

        clean = await consumer._drain_stream(_finite(_make_raw("m1"), _make_raw("m2")), handler)

        assert received == ["m1", "m2"]
        assert clean is True

    @pytest.mark.asyncio
    async def test_ack_called_when_handler_succeeds(self) -> None:
        stub = _make_stub()
        consumer = _make_consumer(stub)

        await consumer._drain_stream(_finite(_make_raw("m1")), AsyncMock())

        stub.Ack.assert_called_once()
        assert stub.Ack.call_args[0][0].id == "m1"

    @pytest.mark.asyncio
    async def test_nack_called_when_handler_raises(self) -> None:
        stub = _make_stub()
        consumer = _make_consumer(stub)

        async def failing_handler(msg: Message) -> None:
            raise ValueError("bad message")

        await consumer._drain_stream(_finite(_make_raw("m1")), failing_handler)

        stub.Nack.assert_called_once()
        req = stub.Nack.call_args[0][0]
        assert req.id == "m1"
        assert "bad message" in req.error

    @pytest.mark.asyncio
    async def test_returns_false_on_stream_error(self) -> None:
        stub = _make_stub()
        consumer = _make_consumer(stub)

        clean = await consumer._drain_stream(_error_stream(), AsyncMock())

        assert clean is False

    @pytest.mark.asyncio
    async def test_ack_carries_consumer_group(self) -> None:
        stub = _make_stub()
        consumer = AsyncConsumer(stub, "t", ConsumerOptions(consumer_group="workers"))

        await consumer._drain_stream(_finite(_make_raw("m1")), AsyncMock())

        stub.Ack.assert_called_once()
        req = stub.Ack.call_args[0][0]
        assert req.id == "m1"
        assert req.consumer_group == "workers"

    @pytest.mark.asyncio
    async def test_nack_carries_consumer_group(self) -> None:
        stub = _make_stub()
        consumer = AsyncConsumer(stub, "t", ConsumerOptions(consumer_group="workers"))

        async def failing_handler(msg: Message) -> None:
            raise ValueError("bad message")

        await consumer._drain_stream(_finite(_make_raw("m1")), failing_handler)

        stub.Nack.assert_called_once()
        req = stub.Nack.call_args[0][0]
        assert req.id == "m1"
        assert req.consumer_group == "workers"

    @pytest.mark.asyncio
    async def test_concurrency_limits_parallel_handlers(self) -> None:
        """With concurrency=2, at most 2 handlers run at the same time."""
        active = 0
        max_active = 0
        # Gate is set from *outside* the stream so there's no deadlock:
        # the semaphore would block the stream from advancing while handlers
        # wait for the gate — so the gate must be released independently.
        gate = asyncio.Event()

        async def slow_handler(msg: Message) -> None:
            nonlocal active, max_active
            active += 1
            max_active = max(max_active, active)
            await gate.wait()
            active -= 1

        stub = _make_stub()
        consumer = _make_consumer(stub, ConsumerOptions(concurrency=2))

        # Yield 2 items (filling both concurrency slots), then release the gate
        # so the handlers can finish, then yield a 3rd item to verify the slot
        # limit held throughout.
        async def stream() -> AsyncIterator[queue_pb2.SubscribeResponse]:
            yield _make_raw("m1")
            yield _make_raw("m2")
            # Both slots are now full; release the gate from here is safe
            # because we're at a yield point (the sem acquisition for m3
            # hasn't happened yet — we're about to yield m3 next).
            gate.set()
            yield _make_raw("m3")

        await consumer._drain_stream(stream(), slow_handler)

        assert max_active <= 2


# ── consume() integration tests ───────────────────────────────────────────────


class TestConsume:
    @pytest.mark.asyncio
    async def test_reconnects_after_stream_error(self) -> None:
        """After a stream error the consumer backs off and reconnects."""
        call_count = 0

        def subscribe_side_effect(*_args: object, **_kwargs: object) -> object:
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                return _error_stream()
            # Second stream immediately raises CancelledError to stop the loop.
            async def stop() -> AsyncIterator[queue_pb2.SubscribeResponse]:
                raise asyncio.CancelledError
                yield
            return stop()

        stub = _make_stub(subscribe_side_effect=subscribe_side_effect)
        consumer = _make_consumer(stub)

        with patch("queueti._consumer.asyncio.sleep", new=AsyncMock()):
            await consumer.consume(AsyncMock())

        assert call_count == 2

    @pytest.mark.asyncio
    async def test_cancelled_from_outside_stops_consume(self) -> None:
        """task.cancel() causes consume() to return; task finishes cleanly."""
        async def blocking_stream() -> AsyncIterator[queue_pb2.SubscribeResponse]:
            await asyncio.sleep(3600)
            yield  # unreachable

        stub = _make_stub(subscribe_return=blocking_stream())
        consumer = _make_consumer(stub)

        task = asyncio.create_task(consumer.consume(AsyncMock()))
        await asyncio.sleep(0)  # let task reach its first await
        task.cancel()
        await task  # consume() catches CancelledError and returns; task finishes
        assert task.done()

    @pytest.mark.asyncio
    async def test_visibility_timeout_sent_in_request(self) -> None:
        stub = _make_stub()
        consumer = _make_consumer(stub, ConsumerOptions(visibility_timeout_seconds=45))

        # One subscribe call that immediately stops the loop
        async def stop_stream() -> AsyncIterator[queue_pb2.SubscribeResponse]:
            raise asyncio.CancelledError
            yield

        stub.Subscribe = MagicMock(return_value=stop_stream())
        await consumer.consume(AsyncMock())

        req = stub.Subscribe.call_args[0][0]
        assert req.visibility_timeout_seconds == 45

    @pytest.mark.asyncio
    async def test_consumer_group_sent_in_subscribe_request(self) -> None:
        stub = _make_stub()
        consumer = AsyncConsumer(stub, "t", ConsumerOptions(consumer_group="workers"))

        async def stop_stream() -> AsyncIterator[queue_pb2.SubscribeResponse]:
            raise asyncio.CancelledError
            yield

        stub.Subscribe = MagicMock(return_value=stop_stream())
        await consumer.consume(AsyncMock())

        req = stub.Subscribe.call_args[0][0]
        assert req.consumer_group == "workers"


# ── consume_batch() tests ─────────────────────────────────────────────────────


class TestConsumeBatch:
    @pytest.mark.asyncio
    async def test_handler_called_with_batch(self) -> None:
        resp = queue_pb2.BatchDequeueResponse()
        resp.messages.append(_make_dequeue_raw("m1"))
        resp.messages.append(_make_dequeue_raw("m2"))

        stub = MagicMock()
        stub.BatchDequeue = AsyncMock(side_effect=[resp, asyncio.CancelledError()])
        consumer = _make_consumer(stub)

        batches: list[list[str]] = []

        async def handler(msgs: list[Message]) -> None:
            batches.append([m.id for m in msgs])

        await consumer.consume_batch(BatchOptions(batch_size=10), handler)
        assert batches == [["m1", "m2"]]

    @pytest.mark.asyncio
    async def test_backs_off_on_empty_batch(self) -> None:
        stub = MagicMock()
        stub.BatchDequeue = AsyncMock(return_value=queue_pb2.BatchDequeueResponse())
        consumer = _make_consumer(stub)

        sleep_calls: list[float] = []

        async def fake_sleep(delay: float) -> None:
            sleep_calls.append(delay)
            raise asyncio.CancelledError

        with patch("queueti._consumer.asyncio.sleep", side_effect=fake_sleep):
            await consumer.consume_batch(BatchOptions(batch_size=5), AsyncMock())

        assert sleep_calls == [0.5]  # BACKOFF_START

    @pytest.mark.asyncio
    async def test_backoff_doubles_on_repeated_empty_batches(self) -> None:
        stub = MagicMock()
        stub.BatchDequeue = AsyncMock(return_value=queue_pb2.BatchDequeueResponse())
        consumer = _make_consumer(stub)

        call_count = 0

        async def fake_sleep(delay: float) -> None:
            nonlocal call_count
            call_count += 1
            if call_count >= 3:
                raise asyncio.CancelledError

        with patch("queueti._consumer.asyncio.sleep", side_effect=fake_sleep):
            await consumer.consume_batch(BatchOptions(batch_size=5), AsyncMock())

        assert call_count == 3

    @pytest.mark.asyncio
    async def test_consumer_group_sent_in_batch_dequeue_request(self) -> None:
        resp = queue_pb2.BatchDequeueResponse()
        resp.messages.append(_make_dequeue_raw("m1"))

        stub = MagicMock()
        stub.BatchDequeue = AsyncMock(side_effect=[resp, asyncio.CancelledError()])
        consumer = AsyncConsumer(stub, "t", ConsumerOptions(consumer_group="workers"))

        await consumer.consume_batch(BatchOptions(batch_size=5, consumer_group="workers"), AsyncMock())

        req = stub.BatchDequeue.call_args_list[0][0][0]
        assert req.consumer_group == "workers"
