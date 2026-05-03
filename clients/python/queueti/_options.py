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
    consumer_group: str = ""

    def __post_init__(self) -> None:
        if self.concurrency < 1:
            raise ValueError(f"concurrency must be >= 1, got {self.concurrency}")


@dataclass
class BatchOptions:
    batch_size: int
    visibility_timeout_seconds: int | None = None
    consumer_group: str = ""

    def __post_init__(self) -> None:
        if self.batch_size < 1:
            raise ValueError(f"batch_size must be >= 1, got {self.batch_size}")
