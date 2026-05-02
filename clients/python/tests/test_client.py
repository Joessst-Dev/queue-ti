from unittest.mock import MagicMock, patch

import pytest

from queueti._client import connect
from queueti._options import ConnectOptions


def _fake_channel() -> MagicMock:
    return MagicMock()


class TestConnect:
    @pytest.mark.asyncio
    async def test_tls_channel_used_by_default(self) -> None:
        with patch("queueti._client.grpc.aio.secure_channel", return_value=_fake_channel()) as secure, \
             patch("queueti._client.grpc.aio.insecure_channel", return_value=_fake_channel()) as insecure:
            await connect("localhost:50051")
        secure.assert_called_once()
        insecure.assert_not_called()

    @pytest.mark.asyncio
    async def test_insecure_channel_when_insecure_true(self) -> None:
        with patch("queueti._client.grpc.aio.secure_channel", return_value=_fake_channel()) as secure, \
             patch("queueti._client.grpc.aio.insecure_channel", return_value=_fake_channel()) as insecure:
            await connect("localhost:50051", options=ConnectOptions(insecure=True))
        insecure.assert_called_once()
        secure.assert_not_called()

    @pytest.mark.asyncio
    async def test_token_uses_secure_channel(self) -> None:
        with patch("queueti._client.grpc.aio.secure_channel", return_value=_fake_channel()) as secure, \
             patch("queueti._client.grpc.aio.insecure_channel", return_value=_fake_channel()) as insecure:
            await connect("localhost:50051", options=ConnectOptions(token="tok"))
        secure.assert_called_once()
        insecure.assert_not_called()

    @pytest.mark.asyncio
    async def test_insecure_with_token_uses_secure_channel(self) -> None:
        """insecure+token: composite credentials with local creds, not insecure_channel."""
        with patch("queueti._client.grpc.aio.secure_channel", return_value=_fake_channel()) as secure, \
             patch("queueti._client.grpc.aio.insecure_channel", return_value=_fake_channel()) as insecure, \
             patch("queueti._client.grpc.local_channel_credentials", return_value=MagicMock()), \
             patch("queueti._client.grpc.metadata_call_credentials", return_value=MagicMock()), \
             patch("queueti._client.grpc.composite_channel_credentials", return_value=MagicMock()):
            await connect("localhost:50051", options=ConnectOptions(insecure=True, token="tok"))
        secure.assert_called_once()
        insecure.assert_not_called()

    @pytest.mark.asyncio
    async def test_returns_async_client(self) -> None:
        from queueti._client import AsyncClient
        with patch("queueti._client.grpc.aio.insecure_channel", return_value=_fake_channel()):
            client = await connect("localhost:50051", options=ConnectOptions(insecure=True))
        assert isinstance(client, AsyncClient)
