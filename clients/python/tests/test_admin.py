from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import httpx
import pytest

from queueti._admin import AdminOptions, AsyncAdminClient, TopicConfig, TopicSchema, TopicStat
from queueti._exceptions import AdminError


def _make_response(
    status_code: int = 200,
    json_body: object = None,
) -> MagicMock:
    resp = MagicMock(spec=httpx.Response)
    resp.status_code = status_code
    resp.is_success = 200 <= status_code < 300
    resp.json.return_value = json_body or {}
    resp.text = ""
    return resp


def _make_client(token: str | None = None) -> AsyncAdminClient:
    return AsyncAdminClient("http://localhost:8080", AdminOptions(token=token))


# ---------------------------------------------------------------------------
# Topic config
# ---------------------------------------------------------------------------


class TestListTopicConfigs:
    @pytest.mark.asyncio
    async def test_returns_list_of_topic_configs(self) -> None:
        client = _make_client()
        payload = {
            "items": [
                {"topic": "orders", "replayable": True, "max_retries": 3,
                 "message_ttl_seconds": None, "max_depth": None,
                 "replay_window_seconds": None, "throughput_limit": None},
            ]
        }
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, payload)
            result = await client.list_topic_configs()

        assert len(result) == 1
        assert result[0] == TopicConfig(topic="orders", replayable=True, max_retries=3)

    @pytest.mark.asyncio
    async def test_returns_empty_list_when_items_missing(self) -> None:
        client = _make_client()
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, {})
            result = await client.list_topic_configs()

        assert result == []


class TestUpsertTopicConfig:
    @pytest.mark.asyncio
    async def test_sends_correct_json_body_and_returns_config(self) -> None:
        client = _make_client()
        config = TopicConfig(topic="orders", replayable=False, max_retries=5)
        response_body = {
            "topic": "orders", "replayable": False, "max_retries": 5,
            "message_ttl_seconds": None, "max_depth": None,
            "replay_window_seconds": None, "throughput_limit": None,
        }
        with patch.object(client._client, "put", new_callable=AsyncMock) as mock_put:
            mock_put.return_value = _make_response(200, response_body)
            result = await client.upsert_topic_config("orders", config)

        mock_put.assert_called_once()
        _, kwargs = mock_put.call_args
        body = kwargs["json"]
        assert body["topic"] == "orders"
        assert body["replayable"] is False
        assert body["max_retries"] == 5
        assert result == TopicConfig(topic="orders", replayable=False, max_retries=5)

    @pytest.mark.asyncio
    async def test_omits_none_fields_from_body(self) -> None:
        client = _make_client()
        config = TopicConfig(topic="t", replayable=True)
        with patch.object(client._client, "put", new_callable=AsyncMock) as mock_put:
            mock_put.return_value = _make_response(200, {"topic": "t", "replayable": True})
            await client.upsert_topic_config("t", config)

        _, kwargs = mock_put.call_args
        body = kwargs["json"]
        assert "max_retries" not in body
        assert "max_depth" not in body


class TestDeleteTopicConfig:
    @pytest.mark.asyncio
    async def test_returns_none_on_204(self) -> None:
        client = _make_client()
        with patch.object(client._client, "delete", new_callable=AsyncMock) as mock_del:
            mock_del.return_value = _make_response(204)
            result = await client.delete_topic_config("orders")

        assert result is None

    @pytest.mark.asyncio
    async def test_raises_admin_error_on_404(self) -> None:
        client = _make_client()
        resp = _make_response(404)
        resp.json.return_value = {"error": "not found"}
        with patch.object(client._client, "delete", new_callable=AsyncMock) as mock_del:
            mock_del.return_value = resp
            with pytest.raises(AdminError) as exc_info:
                await client.delete_topic_config("missing")

        assert exc_info.value.status_code == 404


# ---------------------------------------------------------------------------
# Topic schema
# ---------------------------------------------------------------------------


class TestGetTopicSchema:
    @pytest.mark.asyncio
    async def test_returns_topic_schema(self) -> None:
        client = _make_client()
        payload = {
            "topic": "orders",
            "schema_json": '{"type":"object"}',
            "version": 2,
            "updated_at": "2024-01-01T00:00:00Z",
        }
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, payload)
            result = await client.get_topic_schema("orders")

        assert result == TopicSchema(
            topic="orders",
            schema_json='{"type":"object"}',
            version=2,
            updated_at="2024-01-01T00:00:00Z",
        )

    @pytest.mark.asyncio
    async def test_raises_admin_error_with_404_status_on_missing_topic(self) -> None:
        client = _make_client()
        resp = _make_response(404)
        resp.json.return_value = {"error": "not found"}
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = resp
            with pytest.raises(AdminError) as exc_info:
                await client.get_topic_schema("no-such-topic")

        assert exc_info.value.status_code == 404


class TestUpsertTopicSchema:
    @pytest.mark.asyncio
    async def test_sends_schema_json_body(self) -> None:
        client = _make_client()
        schema = '{"type":"string"}'
        payload = {
            "topic": "events",
            "schema_json": schema,
            "version": 1,
            "updated_at": "2024-06-01T00:00:00Z",
        }
        with patch.object(client._client, "put", new_callable=AsyncMock) as mock_put:
            mock_put.return_value = _make_response(200, payload)
            result = await client.upsert_topic_schema("events", schema)

        _, kwargs = mock_put.call_args
        assert kwargs["json"] == {"schema_json": schema}
        assert result.schema_json == schema
        assert result.version == 1

    @pytest.mark.asyncio
    async def test_returns_topic_schema_dataclass(self) -> None:
        client = _make_client()
        payload = {
            "topic": "t",
            "schema_json": "{}",
            "version": 3,
            "updated_at": "2024-01-01T00:00:00Z",
        }
        with patch.object(client._client, "put", new_callable=AsyncMock) as mock_put:
            mock_put.return_value = _make_response(200, payload)
            result = await client.upsert_topic_schema("t", "{}")

        assert isinstance(result, TopicSchema)


class TestDeleteTopicSchema:
    @pytest.mark.asyncio
    async def test_returns_none_on_204(self) -> None:
        client = _make_client()
        with patch.object(client._client, "delete", new_callable=AsyncMock) as mock_del:
            mock_del.return_value = _make_response(204)
            result = await client.delete_topic_schema("orders")

        assert result is None


class TestListTopicSchemas:
    @pytest.mark.asyncio
    async def test_returns_list_of_schemas(self) -> None:
        client = _make_client()
        payload = {
            "items": [
                {"topic": "a", "schema_json": "{}", "version": 1, "updated_at": "2024-01-01T00:00:00Z"},
                {"topic": "b", "schema_json": "{}", "version": 2, "updated_at": "2024-02-01T00:00:00Z"},
            ]
        }
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, payload)
            result = await client.list_topic_schemas()

        assert len(result) == 2
        assert result[0].topic == "a"
        assert result[1].version == 2


# ---------------------------------------------------------------------------
# Consumer groups
# ---------------------------------------------------------------------------


class TestListConsumerGroups:
    @pytest.mark.asyncio
    async def test_returns_list_of_strings(self) -> None:
        client = _make_client()
        payload = {"items": ["workers", "analytics"]}
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, payload)
            result = await client.list_consumer_groups("orders")

        assert result == ["workers", "analytics"]

    @pytest.mark.asyncio
    async def test_returns_empty_list_when_no_groups(self) -> None:
        client = _make_client()
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, {"items": []})
            result = await client.list_consumer_groups("orders")

        assert result == []


class TestRegisterConsumerGroup:
    @pytest.mark.asyncio
    async def test_sends_correct_body(self) -> None:
        client = _make_client()
        with patch.object(client._client, "post", new_callable=AsyncMock) as mock_post:
            mock_post.return_value = _make_response(201)
            result = await client.register_consumer_group("orders", "workers")

        _, kwargs = mock_post.call_args
        assert kwargs["json"] == {"consumer_group": "workers"}
        assert result is None

    @pytest.mark.asyncio
    async def test_raises_admin_error_with_409_on_conflict(self) -> None:
        client = _make_client()
        resp = _make_response(409)
        resp.json.return_value = {"error": "already exists"}
        with patch.object(client._client, "post", new_callable=AsyncMock) as mock_post:
            mock_post.return_value = resp
            with pytest.raises(AdminError) as exc_info:
                await client.register_consumer_group("orders", "workers")

        assert exc_info.value.status_code == 409


class TestUnregisterConsumerGroup:
    @pytest.mark.asyncio
    async def test_returns_none_on_204(self) -> None:
        client = _make_client()
        with patch.object(client._client, "delete", new_callable=AsyncMock) as mock_del:
            mock_del.return_value = _make_response(204)
            result = await client.unregister_consumer_group("orders", "workers")

        assert result is None

    @pytest.mark.asyncio
    async def test_raises_admin_error_on_non_2xx(self) -> None:
        client = _make_client()
        with patch.object(client._client, "delete", new_callable=AsyncMock) as mock_del:
            mock_del.return_value = _make_response(500)
            with pytest.raises(AdminError) as exc_info:
                await client.unregister_consumer_group("orders", "ghost")

        assert exc_info.value.status_code == 500


# ---------------------------------------------------------------------------
# Stats
# ---------------------------------------------------------------------------


class TestStats:
    @pytest.mark.asyncio
    async def test_returns_list_of_topic_stat(self) -> None:
        client = _make_client()
        payload = {
            "topics": [
                {"topic": "orders", "status": "ready", "count": 42},
                {"topic": "events", "status": "ready", "count": 7},
            ]
        }
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, payload)
            result = await client.stats()

        assert len(result) == 2
        assert result[0] == TopicStat(topic="orders", status="ready", count=42)
        assert result[1].count == 7

    @pytest.mark.asyncio
    async def test_returns_empty_list_when_no_topics(self) -> None:
        client = _make_client()
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(200, {"topics": []})
            result = await client.stats()

        assert result == []

    @pytest.mark.asyncio
    async def test_raises_admin_error_on_server_error(self) -> None:
        client = _make_client()
        with patch.object(client._client, "get", new_callable=AsyncMock) as mock_get:
            mock_get.return_value = _make_response(503)
            with pytest.raises(AdminError) as exc_info:
                await client.stats()

        assert exc_info.value.status_code == 503


# ---------------------------------------------------------------------------
# Auth header
# ---------------------------------------------------------------------------


class TestAuthHeader:
    @pytest.mark.asyncio
    async def test_sets_authorization_header_when_token_provided(self) -> None:
        client = AsyncAdminClient("http://localhost:8080", AdminOptions(token="my-secret"))
        assert client._client.headers.get("authorization") == "Bearer my-secret"

    def test_no_authorization_header_when_no_token(self) -> None:
        client = AsyncAdminClient("http://localhost:8080")
        assert "authorization" not in client._client.headers
