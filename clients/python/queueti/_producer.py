from __future__ import annotations

import grpc

from queueti._exceptions import PublishError
from queueti._options import PublishOptions
from queueti.pb import queue_pb2, queue_pb2_grpc


class AsyncProducer:
    def __init__(self, stub: queue_pb2_grpc.QueueServiceStub) -> None:
        self._stub = stub

    async def publish(
        self,
        topic: str,
        payload: bytes,
        options: PublishOptions | None = None,
    ) -> str:
        """Publish a message and return its assigned ID."""
        metadata = options.metadata if options else {}
        req = queue_pb2.EnqueueRequest(topic=topic, payload=payload, metadata=metadata)
        try:
            resp: queue_pb2.EnqueueResponse = await self._stub.Enqueue(req)
        except grpc.RpcError as exc:
            raise PublishError(f"publish to '{topic}' failed: {exc}") from exc
        return resp.id
