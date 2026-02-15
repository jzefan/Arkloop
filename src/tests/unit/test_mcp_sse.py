from __future__ import annotations

import asyncio
import json

import anyio
import httpx
import pytest

from packages.mcp.client import McpSseClient
from packages.mcp.config import McpServerConfig


class _QueueByteStream(httpx.AsyncByteStream):
    def __init__(self, *, queue: asyncio.Queue[bytes]) -> None:
        self._queue = queue
        self._closed = False

    async def __aiter__(self):
        while True:
            chunk = await self._queue.get()
            if not chunk:
                return
            yield chunk

    async def aclose(self) -> None:
        if self._closed:
            return
        self._closed = True
        self._queue.put_nowait(b"")


class _FakeMcpSseServer:
    def __init__(self) -> None:
        self._queue: asyncio.Queue[bytes] = asyncio.Queue()
        self.seen_auth: list[str | None] = []

    def handle(self, request: httpx.Request) -> httpx.Response:
        self.seen_auth.append(request.headers.get("authorization"))

        if request.method == "GET":
            assert request.url.path == "/sse"
            assert request.headers.get("accept") == "text/event-stream"
            return httpx.Response(
                200,
                headers={"content-type": "text/event-stream"},
                stream=_QueueByteStream(queue=self._queue),
            )

        if request.method == "POST":
            assert request.url.path == "/sse"
            payload = json.loads(request.content.decode("utf-8", errors="replace"))
            if not isinstance(payload, dict):
                return httpx.Response(400)
            self._handle_jsonrpc(payload)
            return httpx.Response(200, json={})

        return httpx.Response(405)

    def _handle_jsonrpc(self, request: dict) -> None:
        method = request.get("method")
        request_id = request.get("id")
        params = request.get("params") or {}

        if method == "initialize":
            protocol_version = params.get("protocolVersion") or "2024-11-05"
            self._send(
                {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": {
                        "protocolVersion": protocol_version,
                        "capabilities": {"tools": {}},
                        "serverInfo": {"name": "fake-mcp-sse", "version": "0"},
                    },
                }
            )
            return

        if method == "notifications/initialized":
            return

        if method == "tools/list":
            tools = [
                {
                    "name": "echo",
                    "title": "Echo",
                    "description": "echo tool",
                    "inputSchema": {
                        "type": "object",
                        "properties": {"text": {"type": "string"}},
                        "required": ["text"],
                        "additionalProperties": False,
                    },
                }
            ]
            self._send({"jsonrpc": "2.0", "id": request_id, "result": {"tools": tools}})
            return

        if method == "tools/call":
            name = params.get("name")
            arguments = params.get("arguments") or {}
            if name != "echo":
                self._send(
                    {"jsonrpc": "2.0", "id": request_id, "error": {"code": -32601, "message": "unknown tool"}}
                )
                return
            self._send(
                {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": {
                        "content": [{"type": "text", "text": str(arguments.get("text", ""))}],
                        "isError": False,
                    },
                }
            )
            return

        self._send({"jsonrpc": "2.0", "id": request_id, "error": {"code": -32601, "message": "unknown method"}})

    def _send(self, payload: dict) -> None:
        raw = json.dumps(payload, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
        frame = f"data: {raw}\n\n".encode("utf-8")
        self._queue.put_nowait(frame)


def test_mcp_server_config_from_json_supports_sse_transport() -> None:
    cfg = McpServerConfig.from_json(
        "remote",
        {
            "transport": "sse",
            "url": "https://example.test",
            "bearerTokenEnv": "TEST_MCP_TOKEN",
            "callTimeoutMs": 1234,
        },
    )
    assert cfg.transport == "sse"
    assert cfg.sse_url == "https://example.test/sse"
    assert cfg.sse_bearer_token_env == "TEST_MCP_TOKEN"
    assert cfg.call_timeout_ms == 1234


def test_mcp_sse_client_can_list_and_call_tool(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("TEST_MCP_TOKEN", "secret-token")
    server = McpServerConfig(
        server_id="test",
        transport="sse",
        sse_url="https://example.test/sse",
        sse_bearer_token_env="TEST_MCP_TOKEN",
        call_timeout_ms=500,
    )

    fake_server = _FakeMcpSseServer()
    transport = httpx.MockTransport(fake_server.handle)

    async def _run() -> None:
        async with httpx.AsyncClient(transport=transport) as http:
            async with McpSseClient(server=server, http=http, reconnect_delay_seconds=0.01) as client:
                tools = await client.list_tools(timeout_ms=500)
                assert [tool.name for tool in tools] == ["echo"]
                result = await client.call_tool(name="echo", arguments={"text": "hi"}, timeout_ms=500)
                assert result.is_error is False
                assert result.content == [{"type": "text", "text": "hi"}]

    anyio.run(_run)

    assert fake_server.seen_auth
    assert all(item == "Bearer secret-token" for item in fake_server.seen_auth)

