from unittest.mock import AsyncMock, MagicMock

import grpc
import pytest

from queueti._exceptions import PublishError
from queueti._options import PublishOptions
from queueti._producer import AsyncProducer
from queueti.pb import queue_pb2


def _make_producer(enqueue_return=None, enqueue_side_effect=None) -> AsyncProducer:
    stub = MagicMock()
    stub.Enqueue = AsyncMock(
        return_value=enqueue_return or queue_pb2.EnqueueResponse(id="msg-1"),
        side_effect=enqueue_side_effect,
    )
    return AsyncProducer(stub)


class TestAsyncProducer:
    @pytest.mark.asyncio
    async def test_publish_returns_message_id(self) -> None:
        producer = _make_producer(enqueue_return=queue_pb2.EnqueueResponse(id="abc-123"))
        msg_id = await producer.publish("my-topic", b"payload")
        assert msg_id == "abc-123"

    @pytest.mark.asyncio
    async def test_publish_sends_correct_request(self) -> None:
        stub = MagicMock()
        stub.Enqueue = AsyncMock(return_value=queue_pb2.EnqueueResponse(id="x"))
        producer = AsyncProducer(stub)

        await producer.publish("orders", b"data", PublishOptions(metadata={"key": "val"}))

        stub.Enqueue.assert_called_once()
        req = stub.Enqueue.call_args[0][0]
        assert req.topic == "orders"
        assert req.payload == b"data"
        assert req.metadata["key"] == "val"

    @pytest.mark.asyncio
    async def test_publish_with_no_metadata(self) -> None:
        stub = MagicMock()
        stub.Enqueue = AsyncMock(return_value=queue_pb2.EnqueueResponse(id="x"))
        producer = AsyncProducer(stub)

        await producer.publish("topic", b"bytes")

        req = stub.Enqueue.call_args[0][0]
        assert dict(req.metadata) == {}

    @pytest.mark.asyncio
    async def test_publish_raises_publish_error_on_rpc_failure(self) -> None:
        error = grpc.RpcError()
        producer = _make_producer(enqueue_side_effect=error)

        with pytest.raises(PublishError, match="my-topic"):
            await producer.publish("my-topic", b"data")
