from __future__ import annotations

import json
from typing import Callable, Awaitable

import httpx


class QueueTiAuth:
    """Handles authentication against the queue-ti admin HTTP API.

    Use the :meth:`login` classmethod to construct an instance.  When the
    server does not require authentication, ``token`` is ``None`` and
    :meth:`refresh` is a no-op.
    """

    def __init__(self, token: str | None, _base: str, _username: str, _password: str) -> None:
        self.token: str | None = token
        self._base = _base
        self._username = _username
        self._password = _password

    @classmethod
    def login(cls, admin_addr: str, username: str, password: str) -> "QueueTiAuth":
        """Check auth status and perform a synchronous login if required.

        :param admin_addr: Base URL of the admin API, e.g. ``"http://localhost:8080"``.
                           A trailing slash is stripped automatically.
        :param username: Username for authentication.
        :param password: Password for authentication.
        :returns: A :class:`QueueTiAuth` instance.
        :raises httpx.HTTPError: On network or HTTP errors.
        """
        base = admin_addr.rstrip("/")
        with httpx.Client() as client:
            status_resp = client.get(f"{base}/api/auth/status")
            status_resp.raise_for_status()
            auth_required = status_resp.json().get("auth_required", False)

            if not auth_required:
                return cls(None, base, username, password)

            token = cls._do_login(client, base, username, password)
            return cls(token, base, username, password)

    @staticmethod
    def _do_login(client: httpx.Client, base: str, username: str, password: str) -> str:
        resp = client.post(
            f"{base}/api/auth/login",
            content=json.dumps({"username": username, "password": password}),
            headers={"Content-Type": "application/json"},
        )
        resp.raise_for_status()
        token = resp.json().get("token", "")
        if not token:
            raise ValueError("queue-ti auth: server returned empty token")
        return str(token)

    def refresh(self) -> str:
        """Re-authenticate and return the new token.

        Compatible with the sync ``token_refresher`` hook.  When auth is
        disabled (``token`` is ``None``), this is a no-op and returns ``""``.
        """
        if self.token is None:
            return ""
        with httpx.Client() as client:
            self.token = self._do_login(client, self._base, self._username, self._password)
        return self.token

    async def async_refresh(self) -> str:
        """Re-authenticate asynchronously and return the new token.

        Compatible with the async ``token_refresher`` hook.  When auth is
        disabled (``token`` is ``None``), this is a no-op and returns ``""``.
        """
        if self.token is None:
            return ""
        async with httpx.AsyncClient() as client:
            resp = await client.post(
                f"{self._base}/api/auth/login",
                content=json.dumps({"username": self._username, "password": self._password}),
                headers={"Content-Type": "application/json"},
            )
            resp.raise_for_status()
            token = resp.json().get("token", "")
            if not token:
                raise ValueError("queue-ti auth: server returned empty token")
            self.token = str(token)
        return self.token
