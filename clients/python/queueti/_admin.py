from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import httpx

from queueti._exceptions import AdminError


@dataclass
class AdminOptions:
    token: str | None = None


@dataclass
class TopicConfig:
    topic: str
    replayable: bool
    max_retries: int | None = None
    message_ttl_seconds: int | None = None
    max_depth: int | None = None
    replay_window_seconds: int | None = None
    throughput_limit: int | None = None


@dataclass
class TopicSchema:
    topic: str
    schema_json: str
    version: int
    updated_at: str


@dataclass
class TopicStat:
    topic: str
    status: str
    count: int


def _topic_config_from_dict(d: dict[str, Any]) -> TopicConfig:
    return TopicConfig(
        topic=str(d["topic"]),
        replayable=bool(d["replayable"]),
        max_retries=int(d["max_retries"]) if d.get("max_retries") is not None else None,
        message_ttl_seconds=int(d["message_ttl_seconds"]) if d.get("message_ttl_seconds") is not None else None,
        max_depth=int(d["max_depth"]) if d.get("max_depth") is not None else None,
        replay_window_seconds=int(d["replay_window_seconds"]) if d.get("replay_window_seconds") is not None else None,
        throughput_limit=int(d["throughput_limit"]) if d.get("throughput_limit") is not None else None,
    )


def _topic_schema_from_dict(d: dict[str, Any]) -> TopicSchema:
    return TopicSchema(
        topic=str(d["topic"]),
        schema_json=str(d["schema_json"]),
        version=int(d["version"]),        updated_at=str(d["updated_at"]),
    )


def _topic_stat_from_dict(d: dict[str, Any]) -> TopicStat:
    return TopicStat(
        topic=str(d["topic"]),
        status=str(d["status"]),
        count=int(d["count"]),    )


class AsyncAdminClient:
    """Async HTTP client for the queue-ti admin REST API (port 8080).

    :param base_url: Base URL of the admin API, e.g. ``"http://localhost:8080"``.
    :param options: Optional :class:`AdminOptions` for auth.
    """

    def __init__(self, base_url: str, options: AdminOptions | None = None) -> None:
        self._base_url = base_url.rstrip("/")
        headers: dict[str, str] = {}
        if options and options.token:
            headers["Authorization"] = f"Bearer {options.token}"
        self._client = httpx.AsyncClient(base_url=self._base_url, headers=headers)

    async def close(self) -> None:
        """Close the underlying HTTP client."""
        await self._client.aclose()

    async def __aenter__(self) -> "AsyncAdminClient":
        return self

    async def __aexit__(self, *_: object) -> None:
        await self.close()

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _raise_for_status(self, response: httpx.Response) -> None:
        if response.is_success:
            return
        try:
            detail = response.json().get("error") or response.text
        except Exception:
            detail = response.text
        raise AdminError(
            f"admin API returned {response.status_code}: {detail}",
            status_code=response.status_code,
        )

    # ------------------------------------------------------------------
    # Topic config
    # ------------------------------------------------------------------

    async def list_topic_configs(self) -> list[TopicConfig]:
        """GET /api/topic-configs — returns all topic configurations."""
        resp = await self._client.get("/api/topic-configs")
        self._raise_for_status(resp)
        data: dict[str, Any] = resp.json()
        items = data.get("items") or []
        return [_topic_config_from_dict(item) for item in items]
    async def upsert_topic_config(self, topic: str, config: TopicConfig) -> TopicConfig:
        """PUT /api/topic-configs/{topic} — create or replace a topic configuration."""
        body: dict[str, Any] = {
            "topic": config.topic,
            "replayable": config.replayable,
        }
        if config.max_retries is not None:
            body["max_retries"] = config.max_retries
        if config.message_ttl_seconds is not None:
            body["message_ttl_seconds"] = config.message_ttl_seconds
        if config.max_depth is not None:
            body["max_depth"] = config.max_depth
        if config.replay_window_seconds is not None:
            body["replay_window_seconds"] = config.replay_window_seconds
        if config.throughput_limit is not None:
            body["throughput_limit"] = config.throughput_limit

        resp = await self._client.put(f"/api/topic-configs/{topic}", json=body)
        self._raise_for_status(resp)
        return _topic_config_from_dict(resp.json())

    async def delete_topic_config(self, topic: str) -> None:
        """DELETE /api/topic-configs/{topic} — remove a topic configuration."""
        resp = await self._client.delete(f"/api/topic-configs/{topic}")
        self._raise_for_status(resp)

    # ------------------------------------------------------------------
    # Topic schema
    # ------------------------------------------------------------------

    async def list_topic_schemas(self) -> list[TopicSchema]:
        """GET /api/topic-schemas — returns all registered topic schemas."""
        resp = await self._client.get("/api/topic-schemas")
        self._raise_for_status(resp)
        data: dict[str, Any] = resp.json()
        items = data.get("items") or []
        return [_topic_schema_from_dict(item) for item in items]
    async def get_topic_schema(self, topic: str) -> TopicSchema:
        """GET /api/topic-schemas/{topic} — fetch a single topic's schema."""
        resp = await self._client.get(f"/api/topic-schemas/{topic}")
        self._raise_for_status(resp)
        return _topic_schema_from_dict(resp.json())

    async def upsert_topic_schema(self, topic: str, schema_json: str) -> TopicSchema:
        """PUT /api/topic-schemas/{topic} — register or replace a topic schema."""
        resp = await self._client.put(
            f"/api/topic-schemas/{topic}",
            json={"schema_json": schema_json},
        )
        self._raise_for_status(resp)
        return _topic_schema_from_dict(resp.json())

    async def delete_topic_schema(self, topic: str) -> None:
        """DELETE /api/topic-schemas/{topic} — remove a topic schema."""
        resp = await self._client.delete(f"/api/topic-schemas/{topic}")
        self._raise_for_status(resp)

    # ------------------------------------------------------------------
    # Consumer groups
    # ------------------------------------------------------------------

    async def list_consumer_groups(self, topic: str) -> list[str]:
        """GET /api/topics/{topic}/consumer-groups — list registered consumer groups."""
        resp = await self._client.get(f"/api/topics/{topic}/consumer-groups")
        self._raise_for_status(resp)
        data: dict[str, Any] = resp.json()
        items = data.get("items") or []
        return [str(g) for g in items]
    async def register_consumer_group(self, topic: str, group: str) -> None:
        """POST /api/topics/{topic}/consumer-groups — register a consumer group."""
        resp = await self._client.post(
            f"/api/topics/{topic}/consumer-groups",
            json={"consumer_group": group},
        )
        self._raise_for_status(resp)

    async def unregister_consumer_group(self, topic: str, group: str) -> None:
        """DELETE /api/topics/{topic}/consumer-groups/{group} — remove a consumer group."""
        resp = await self._client.delete(f"/api/topics/{topic}/consumer-groups/{group}")
        self._raise_for_status(resp)

    # ------------------------------------------------------------------
    # Stats
    # ------------------------------------------------------------------

    async def stats(self) -> list[TopicStat]:
        """GET /api/stats — return per-topic message statistics."""
        resp = await self._client.get("/api/stats")
        self._raise_for_status(resp)
        data: dict[str, Any] = resp.json()
        topics = data.get("topics") or []
        return [_topic_stat_from_dict(t) for t in topics]