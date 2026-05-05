from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from queueti._auth import QueueTiAuth


def _make_response(status_code: int = 200, body: object = None) -> MagicMock:
    resp = MagicMock(spec=httpx.Response)
    resp.status_code = status_code
    resp.is_success = 200 <= status_code < 300
    resp.json.return_value = body or {}
    resp.raise_for_status = MagicMock()
    if not resp.is_success:
        resp.raise_for_status.side_effect = httpx.HTTPStatusError(
            message=f"HTTP {status_code}",
            request=MagicMock(),
            response=resp,
        )
    return resp


class TestQueueTiAuthLogin:
    def test_no_auth_required_returns_none_token(self) -> None:
        status_resp = _make_response(body={"auth_required": False})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp

            auth = QueueTiAuth.login("http://localhost:8080", "user", "pass")

        assert auth.token is None
        mock_client.post.assert_not_called()

    def test_no_auth_required_strips_trailing_slash(self) -> None:
        status_resp = _make_response(body={"auth_required": False})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp

            QueueTiAuth.login("http://localhost:8080/", "u", "p")

        mock_client.get.assert_called_once_with("http://localhost:8080/api/auth/status")

    def test_auth_required_returns_token(self) -> None:
        status_resp = _make_response(body={"auth_required": True})
        login_resp = _make_response(body={"token": "jwt-abc"})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            mock_client.post.return_value = login_resp

            auth = QueueTiAuth.login("http://localhost:8080", "admin", "secret")

        assert auth.token == "jwt-abc"

    def test_auth_required_sends_json_encoded_credentials(self) -> None:
        status_resp = _make_response(body={"auth_required": True})
        login_resp = _make_response(body={"token": "tok"})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            mock_client.post.return_value = login_resp

            QueueTiAuth.login("http://localhost:8080", 'user"name', 'p\\a"ss')

        _, kwargs = mock_client.post.call_args
        body = json.loads(kwargs["content"])
        assert body["username"] == 'user"name'
        assert body["password"] == 'p\\a"ss'

    def test_login_failure_raises(self) -> None:
        status_resp = _make_response(body={"auth_required": True})
        login_resp = _make_response(status_code=401, body={"error": "invalid credentials"})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            mock_client.post.return_value = login_resp

            with pytest.raises(httpx.HTTPStatusError):
                QueueTiAuth.login("http://localhost:8080", "bad", "creds")

    def test_empty_token_in_response_raises(self) -> None:
        status_resp = _make_response(body={"auth_required": True})
        login_resp = _make_response(body={"token": ""})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            mock_client.post.return_value = login_resp

            with pytest.raises(ValueError, match="empty token"):
                QueueTiAuth.login("http://localhost:8080", "u", "p")


class TestQueueTiAuthRefresh:
    def test_refresh_is_noop_when_auth_disabled(self) -> None:
        status_resp = _make_response(body={"auth_required": False})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            auth = QueueTiAuth.login("http://localhost:8080", "u", "p")

        result = auth.refresh()
        assert result == ""
        assert auth.token is None

    def test_refresh_reauthenticates_and_updates_token(self) -> None:
        status_resp = _make_response(body={"auth_required": True})
        login_resp = _make_response(body={"token": "initial"})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            mock_client.post.return_value = login_resp
            auth = QueueTiAuth.login("http://localhost:8080", "admin", "secret")
        assert auth.token == "initial"

        refresh_resp = _make_response(body={"token": "refreshed"})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.post.return_value = refresh_resp
            result = auth.refresh()

        assert result == "refreshed"
        assert auth.token == "refreshed"


@pytest.mark.asyncio
class TestQueueTiAuthAsyncRefresh:
    async def test_async_refresh_is_noop_when_auth_disabled(self) -> None:
        status_resp = _make_response(body={"auth_required": False})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            auth = QueueTiAuth.login("http://localhost:8080", "u", "p")

        result = await auth.async_refresh()
        assert result == ""

    async def test_async_refresh_reauthenticates(self) -> None:
        status_resp = _make_response(body={"auth_required": True})
        login_resp = _make_response(body={"token": "initial"})
        with patch("httpx.Client") as mock_client_cls:
            mock_client = mock_client_cls.return_value.__enter__.return_value
            mock_client.get.return_value = status_resp
            mock_client.post.return_value = login_resp
            auth = QueueTiAuth.login("http://localhost:8080", "admin", "secret")

        refresh_resp = MagicMock(spec=httpx.Response)
        refresh_resp.json.return_value = {"token": "async-refreshed"}
        refresh_resp.raise_for_status = MagicMock()

        with patch("httpx.AsyncClient") as mock_async_cls:
            mock_async_client = mock_async_cls.return_value.__aenter__.return_value
            mock_async_client.post = AsyncMock(return_value=refresh_resp)
            result = await auth.async_refresh()

        assert result == "async-refreshed"
        assert auth.token == "async-refreshed"
