class QueueTiError(Exception):
    """Base exception for all queue-ti client errors."""


class PublishError(QueueTiError):
    """Raised when a message cannot be published."""


class AckError(QueueTiError):
    """Raised when an ack RPC fails."""


class NackError(QueueTiError):
    """Raised when a nack RPC fails."""


class AdminError(QueueTiError):
    """Raised when an admin HTTP request fails.

    :param status_code: HTTP status code returned by the server.
    """

    def __init__(self, message: str, status_code: int) -> None:
        super().__init__(message)
        self.status_code = status_code
