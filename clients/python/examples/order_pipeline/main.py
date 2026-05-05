"""
Order pipeline — demonstrates the full producer → consumer → ack lifecycle
against a local queue-ti instance.

Run: python main.py  (requires docker-compose up)
"""

from __future__ import annotations

import asyncio
import dataclasses
import json
import logging
import signal
import sys

import httpx

from queueti import (
    AsyncAdminClient,
    AdminError,
    BatchOptions,
    ConnectOptions,
    ConsumerOptions,
    Message,
    PublishOptions,
    connect,
)

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(message)s")
log = logging.getLogger(__name__)

GRPC_ADDR = "localhost:50051"
ADMIN_URL = "http://localhost:8080"
TOPIC = "orders"
DLQ_TOPIC = "orders.dlq"
CONSUMER_GROUP = "fulfillment"


@dataclasses.dataclass
class Order:
    id: str
    item: str
    amount: int
    poison: bool = False


ORDERS = [
    Order("ord-1", "Widget A", 2),
    Order("ord-2", "Gadget B", 1),
    Order("ord-3", "poison", 0, poison=True),
    Order("ord-4", "Widget C", 5),
    Order("ord-5", "Gadget D", 3),
]


async def register_consumer_group() -> None:
    async with AsyncAdminClient(ADMIN_URL) as admin:
        try:
            await admin.register_consumer_group(TOPIC, CONSUMER_GROUP)
            log.info("consumer group %r registered", CONSUMER_GROUP)
        except AdminError as exc:
            if exc.status_code == 409:
                log.info("consumer group %r already exists", CONSUMER_GROUP)
            else:
                raise


async def produce(stop: asyncio.Event) -> None:
    client = await connect(GRPC_ADDR, ConnectOptions(insecure=True))
    producer = client.producer()

    for order in ORDERS:
        if stop.is_set():
            break
        await asyncio.sleep(0.5)
        payload = json.dumps(dataclasses.asdict(order)).encode()
        msg_id = await producer.publish(
            TOPIC,
            payload,
            PublishOptions(metadata={"source": "order-pipeline"}),
        )
        log.info("published %s → message %s", order.id, msg_id)

    await client.close()


async def consume(stop: asyncio.Event) -> None:
    client = await connect(GRPC_ADDR, ConnectOptions(insecure=True))
    consumer = client.consumer(
        TOPIC,
        ConsumerOptions(consumer_group=CONSUMER_GROUP, concurrency=3),
    )
    log.info("consuming from %r (group %r) — Ctrl-C to stop", TOPIC, CONSUMER_GROUP)

    async def handler(msg: Message) -> None:
        order = Order(**json.loads(msg.payload))

        if order.poison:
            log.info("nack %s: poison pill detected (retry %d)", msg.id, msg.retry_count)
            await msg.nack("poison pill")
            return

        log.info("ack %s: processed order %s — %d×%s", msg.id, order.id, order.amount, order.item)
        await msg.ack()

    consume_task = asyncio.create_task(consumer.consume(handler))
    await stop.wait()
    consume_task.cancel()
    try:
        await consume_task
    except asyncio.CancelledError:
        pass

    await client.close()


async def drain_dlq() -> None:
    client = await connect(GRPC_ADDR, ConnectOptions(insecure=True))
    consumer = client.consumer(
        DLQ_TOPIC,
        ConsumerOptions(consumer_group=CONSUMER_GROUP),
    )
    log.info("draining DLQ %r", DLQ_TOPIC)

    async def dlq_handler(messages: list[Message]) -> None:
        for msg in messages:
            log.info("[DLQ] %s retry=%d payload=%s", msg.id, msg.retry_count, msg.payload)
            await msg.ack()

    await consumer.consume_batch(BatchOptions(batch_size=10, consumer_group=CONSUMER_GROUP), dlq_handler)
    await client.close()


async def main() -> None:
    stop = asyncio.Event()

    def _shutdown() -> None:
        log.info("shutting down…")
        stop.set()

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, _shutdown)

    await register_consumer_group()

    async with asyncio.TaskGroup() as tg:
        tg.create_task(produce(stop))
        tg.create_task(drain_dlq())
        tg.create_task(consume(stop))


if __name__ == "__main__":
    asyncio.run(main())
