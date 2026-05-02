from dataclasses import dataclass, field
from typing import Awaitable, Callable

TokenRefresher = Callable[[], Awaitable[str]]


@dataclass
class ConnectOptions:
    token: str | None = None
    token_refresher: TokenRefresher | None = None
    insecure: bool = False


@dataclass
class PublishOptions:
    metadata: dict[str, str] = field(default_factory=dict)


@dataclass
class ConsumerOptions:
    concurrency: int = 1
    visibility_timeout_seconds: int | None = None


@dataclass
class BatchOptions:
    batch_size: int
    visibility_timeout_seconds: int | None = None
