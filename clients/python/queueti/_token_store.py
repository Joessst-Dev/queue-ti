from __future__ import annotations

import base64
import json
import threading
import time


class TokenStore:
    """Thread-safe holder for the current bearer token."""

    def __init__(self, token: str) -> None:
        self._token = token
        self._lock = threading.Lock()

    def get(self) -> str:
        with self._lock:
            return self._token

    def set(self, token: str) -> None:
        with self._lock:
            self._token = token


def parse_token_expiry(token: str) -> float:
    """Return the JWT ``exp`` claim as a Unix timestamp.

    Raises ``ValueError`` if the token is not a valid JWT or has no ``exp``.
    """
    parts = token.split(".")
    if len(parts) != 3:
        raise ValueError("not a valid JWT")
    # Add padding so base64 doesn't complain about missing '='
    payload = parts[1] + "=="
    try:
        data = json.loads(base64.urlsafe_b64decode(payload))
    except Exception as exc:
        raise ValueError(f"cannot decode JWT payload: {exc}") from exc
    if "exp" not in data:
        raise ValueError("JWT has no exp claim")
    return float(data["exp"])


def seconds_until_expiry(token: str, advance_seconds: float = 60.0) -> float:
    """Seconds until the token expires minus the advance window. Negative means refresh now."""
    exp = parse_token_expiry(token)
    return exp - time.time() - advance_seconds
