class QueueTiError(Exception):
    """Base exception for all queue-ti client errors."""


class PublishError(QueueTiError):
    """Raised when a message cannot be published."""


class AckError(QueueTiError):
    """Raised when an ack RPC fails."""


class NackError(QueueTiError):
    """Raised when a nack RPC fails."""
