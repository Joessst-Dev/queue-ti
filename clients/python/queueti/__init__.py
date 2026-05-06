"""queue-ti Python client library."""

from queueti._auth import QueueTiAuth
from queueti._admin import AdminOptions, AsyncAdminClient, TopicConfig, TopicSchema, TopicStat
from queueti._client import AsyncClient, connect
from queueti._consumer import AsyncConsumer, BatchHandler, MessageHandler
from queueti._exceptions import AckError, AdminError, NackError, PublishError, QueueTiError
from queueti._message import Message, SyncMessage
from queueti._options import BatchOptions, ConnectOptions, ConsumerOptions, PublishOptions, TLSOptions
from queueti._producer import AsyncProducer
from queueti._sync import Client, Consumer, Producer, connect_sync

__all__ = [
    # Auth
    "QueueTiAuth",
    # Async API
    "connect",
    "AsyncClient",
    "AsyncProducer",
    "AsyncConsumer",
    "Message",
    "MessageHandler",
    "BatchHandler",
    # Admin API
    "AsyncAdminClient",
    "AdminOptions",
    "TopicConfig",
    "TopicSchema",
    "TopicStat",
    # Sync API
    "connect_sync",
    "Client",
    "Producer",
    "Consumer",
    "SyncMessage",
    # Options
    "ConnectOptions",
    "TLSOptions",
    "PublishOptions",
    "ConsumerOptions",
    "BatchOptions",
    # Exceptions
    "QueueTiError",
    "PublishError",
    "AckError",
    "NackError",
    "AdminError",
]
