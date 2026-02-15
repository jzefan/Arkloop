from __future__ import annotations

import asyncio
from collections import deque
from dataclasses import dataclass, field
import json
import os
from typing import Any, Mapping

from .config import McpServerConfig

_SUPPORTED_PROTOCOL_VERSIONS: tuple[str, ...] = (
    "2025-06-18",
    "2025-03-26",
    "2024-11-05",
)

_DEFAULT_PROTOCOL_VERSION = _SUPPORTED_PROTOCOL_VERSIONS[0]

_DEFAULT_CLIENT_NAME = "arkloop"
_DEFAULT_CLIENT_VERSION = "0"

_RPC_VERSION = "2.0"

_STDIO_ENCODING = "utf-8"

_DEFAULT_STDERROUT_TAIL_LINES = 50


@dataclass(frozen=True, slots=True)
class McpTool:
    name: str
    title: str | None = None
    description: str | None = None
    input_schema: Mapping[str, Any] = field(default_factory=dict)


@dataclass(frozen=True, slots=True)
class McpToolCallResult:
    content: list[Mapping[str, Any]] = field(default_factory=list)
    is_error: bool = False


class McpClientError(RuntimeError):
    pass


class McpTimeoutError(McpClientError):
    pass


class McpDisconnectedError(McpClientError):
    pass


class McpRpcError(McpClientError):
    def __init__(self, *, code: int | None, message: str, data: Any | None = None) -> None:
        super().__init__(message)
        self.code = code
        self.data = data


class McpStdioClient:
    def __init__(
        self,
        *,
        server: McpServerConfig,
        protocol_version: str = _DEFAULT_PROTOCOL_VERSION,
        client_name: str = _DEFAULT_CLIENT_NAME,
        client_version: str = _DEFAULT_CLIENT_VERSION,
        stderr_tail_lines: int = _DEFAULT_STDERROUT_TAIL_LINES,
    ) -> None:
        if protocol_version not in _SUPPORTED_PROTOCOL_VERSIONS:
            raise ValueError(f"不支持的 MCP protocol_version: {protocol_version}")
        self._server = server
        self._requested_protocol_version = protocol_version
        self._client_name = client_name
        self._client_version = client_version

        self._process: asyncio.subprocess.Process | None = None
        self._stdout_task: asyncio.Task[None] | None = None
        self._stderr_task: asyncio.Task[None] | None = None

        self._pending: dict[int, asyncio.Future[Mapping[str, Any]]] = {}
        self._pending_lock = asyncio.Lock()
        self._write_lock = asyncio.Lock()
        self._next_id = 1

        self._stderr_tail = deque(maxlen=max(1, int(stderr_tail_lines)))
        self._initialized = False
        self._negotiated_protocol_version: str | None = None

    async def __aenter__(self) -> "McpStdioClient":
        await self.start()
        return self

    async def __aexit__(self, exc_type, exc, tb) -> None:
        await self.close()

    @property
    def negotiated_protocol_version(self) -> str | None:
        return self._negotiated_protocol_version

    async def start(self) -> None:
        if self._process is not None:
            return

        env = _build_server_env(self._server)

        self._process = await asyncio.create_subprocess_exec(
            self._server.command,
            *self._server.args,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=self._server.cwd,
            env=env,
        )

        self._stdout_task = asyncio.create_task(self._stdout_loop())
        self._stderr_task = asyncio.create_task(self._stderr_loop())

    async def close(self) -> None:
        tasks = [task for task in (self._stdout_task, self._stderr_task) if task is not None]
        for task in tasks:
            task.cancel()

        process = self._process
        if process is not None:
            if process.stdin is not None:
                try:
                    process.stdin.close()
                except Exception:
                    pass

            if process.returncode is None:
                try:
                    process.terminate()
                except ProcessLookupError:
                    pass
                try:
                    await asyncio.wait_for(process.wait(), timeout=1.0)
                except asyncio.TimeoutError:
                    try:
                        process.kill()
                    except ProcessLookupError:
                        pass
                    try:
                        await asyncio.wait_for(process.wait(), timeout=1.0)
                    except asyncio.TimeoutError:
                        pass

        for task in tasks:
            try:
                await task
            except asyncio.CancelledError:
                pass
            except Exception:
                pass

        self._process = None
        self._stdout_task = None
        self._stderr_task = None

        async with self._pending_lock:
            pending = list(self._pending.values())
            self._pending.clear()
        for fut in pending:
            if fut.done():
                continue
            fut.set_exception(McpDisconnectedError("MCP client 已关闭"))

    async def initialize(self, *, timeout_ms: int | None = None) -> None:
        if self._initialized:
            return
        await self.start()

        request_timeout_ms = _coalesce_timeout_ms(timeout_ms, self._server.call_timeout_ms)
        result = await self._request(
            method="initialize",
            params={
                "protocolVersion": self._requested_protocol_version,
                "capabilities": {},
                "clientInfo": {"name": self._client_name, "version": self._client_version},
            },
            timeout_ms=request_timeout_ms,
        )
        protocol_version = result.get("protocolVersion")
        if not isinstance(protocol_version, str) or not protocol_version.strip():
            raise McpClientError("initialize 返回缺少 protocolVersion")
        cleaned_protocol_version = protocol_version.strip()
        if cleaned_protocol_version not in _SUPPORTED_PROTOCOL_VERSIONS:
            raise McpClientError(f"不支持的 MCP 协议版本: {cleaned_protocol_version}")
        self._negotiated_protocol_version = cleaned_protocol_version

        await self._notify(method="notifications/initialized", params=None)
        self._initialized = True

    async def list_tools(self, *, timeout_ms: int | None = None) -> tuple[McpTool, ...]:
        await self.initialize(timeout_ms=timeout_ms)
        request_timeout_ms = _coalesce_timeout_ms(timeout_ms, self._server.call_timeout_ms)
        result = await self._request(method="tools/list", params={}, timeout_ms=request_timeout_ms)
        raw_tools = result.get("tools")
        if raw_tools is None:
            return ()
        if not isinstance(raw_tools, list):
            raise McpClientError("tools/list 返回 tools 不是数组")

        tools: list[McpTool] = []
        for raw in raw_tools:
            if not isinstance(raw, Mapping):
                continue
            name = raw.get("name")
            if not isinstance(name, str) or not name.strip():
                continue
            title = raw.get("title")
            title_value = title.strip() if isinstance(title, str) and title.strip() else None
            description = raw.get("description")
            desc_value = description.strip() if isinstance(description, str) and description.strip() else None
            input_schema = raw.get("inputSchema") or {}
            schema_value = dict(input_schema) if isinstance(input_schema, Mapping) else {}
            tools.append(
                McpTool(
                    name=name.strip(),
                    title=title_value,
                    description=desc_value,
                    input_schema=schema_value,
                )
            )
        return tuple(tools)

    async def call_tool(
        self,
        *,
        name: str,
        arguments: Mapping[str, Any],
        timeout_ms: int | None = None,
    ) -> McpToolCallResult:
        await self.initialize(timeout_ms=timeout_ms)
        request_timeout_ms = _coalesce_timeout_ms(timeout_ms, self._server.call_timeout_ms)
        result = await self._request(
            method="tools/call",
            params={"name": name, "arguments": dict(arguments)},
            timeout_ms=request_timeout_ms,
        )
        raw_content = result.get("content") or []
        if not isinstance(raw_content, list):
            raise McpClientError("tools/call 返回 content 不是数组")
        content: list[Mapping[str, Any]] = []
        for item in raw_content:
            if not isinstance(item, Mapping):
                continue
            content.append(dict(item))
        is_error = bool(result.get("isError"))
        return McpToolCallResult(content=content, is_error=is_error)

    async def _notify(self, *, method: str, params: Mapping[str, Any] | None) -> None:
        await self.start()
        payload: dict[str, Any] = {"jsonrpc": _RPC_VERSION, "method": method}
        if params is not None:
            payload["params"] = dict(params)
        await self._write_json(payload)

    async def _request(
        self,
        *,
        method: str,
        params: Mapping[str, Any] | None,
        timeout_ms: int,
    ) -> Mapping[str, Any]:
        await self.start()
        request_id = self._allocate_request_id()
        loop = asyncio.get_running_loop()
        fut: asyncio.Future[Mapping[str, Any]] = loop.create_future()

        async with self._pending_lock:
            self._pending[request_id] = fut

        payload: dict[str, Any] = {"jsonrpc": _RPC_VERSION, "id": request_id, "method": method}
        if params is not None:
            payload["params"] = dict(params)

        try:
            await self._write_json(payload)
        except Exception:
            async with self._pending_lock:
                self._pending.pop(request_id, None)
            raise

        try:
            return await asyncio.wait_for(fut, timeout=timeout_ms / 1000.0)
        except asyncio.TimeoutError as exc:
            async with self._pending_lock:
                self._pending.pop(request_id, None)
            raise McpTimeoutError(f"MCP 请求超时: {method}") from exc

    def _allocate_request_id(self) -> int:
        request_id = self._next_id
        self._next_id += 1
        return request_id

    async def _write_json(self, payload: Mapping[str, Any]) -> None:
        process = self._process
        if process is None or process.stdin is None:
            raise McpDisconnectedError("MCP 进程未启动")
        line = json.dumps(payload, ensure_ascii=False, separators=(",", ":"), sort_keys=True) + "\n"
        data = line.encode(_STDIO_ENCODING)
        async with self._write_lock:
            try:
                process.stdin.write(data)
                await process.stdin.drain()
            except (BrokenPipeError, ConnectionResetError) as exc:
                raise McpDisconnectedError("MCP stdin 已断开") from exc

    async def _stdout_loop(self) -> None:
        process = self._process
        if process is None or process.stdout is None:
            return

        try:
            while True:
                line = await process.stdout.readline()
                if not line:
                    break
                message = _parse_json_line(line)
                if message is None:
                    continue
                await self._handle_message(message)
        except asyncio.CancelledError:
            raise
        except Exception:
            pass
        finally:
            await self._fail_pending(McpDisconnectedError("MCP stdout 已结束"))

    async def _stderr_loop(self) -> None:
        process = self._process
        if process is None or process.stderr is None:
            return

        try:
            while True:
                line = await process.stderr.readline()
                if not line:
                    break
                text = line.decode(_STDIO_ENCODING, errors="replace").rstrip("\r\n")
                if text:
                    self._stderr_tail.append(text)
        except asyncio.CancelledError:
            raise
        except Exception:
            pass

    async def _handle_message(self, message: Mapping[str, Any]) -> None:
        raw_id = message.get("id")
        if not isinstance(raw_id, int):
            return

        fut: asyncio.Future[Mapping[str, Any]] | None = None
        async with self._pending_lock:
            fut = self._pending.pop(raw_id, None)

        if fut is None or fut.done():
            return

        error = message.get("error")
        if isinstance(error, Mapping):
            fut.set_exception(_rpc_error_from_mapping(error))
            return

        result = message.get("result")
        if isinstance(result, Mapping):
            fut.set_result(dict(result))
            return

        fut.set_exception(McpClientError("MCP 响应缺少 result/error"))

    async def _fail_pending(self, error: Exception) -> None:
        async with self._pending_lock:
            pending = list(self._pending.values())
            self._pending.clear()
        details = _stderr_tail_to_string(self._stderr_tail)
        if details:
            error = McpDisconnectedError(f"{error}，stderr 尾部：{details}")
        for fut in pending:
            if fut.done():
                continue
            fut.set_exception(error)


def _parse_json_line(raw: bytes) -> Mapping[str, Any] | None:
    text = raw.decode(_STDIO_ENCODING, errors="replace").strip()
    if not text:
        return None
    try:
        data = json.loads(text)
    except json.JSONDecodeError:
        return None
    return data if isinstance(data, Mapping) else None


def _rpc_error_from_mapping(error: Mapping[str, Any]) -> McpRpcError:
    code = error.get("code")
    code_value = int(code) if isinstance(code, int) else None
    message = error.get("message")
    message_value = message.strip() if isinstance(message, str) and message.strip() else "MCP RPC 错误"
    data = error.get("data")
    return McpRpcError(code=code_value, message=message_value, data=data)


def _stderr_tail_to_string(lines: deque[str]) -> str:
    if not lines:
        return ""
    joined = " | ".join(list(lines))
    return joined[:8000]


def _coalesce_timeout_ms(value: int | None, fallback: int) -> int:
    if value is None:
        return fallback
    if value <= 0:
        return fallback
    return value


def _build_server_env(server: McpServerConfig) -> dict[str, str] | None:
    if server.inherit_parent_env:
        merged = dict(os.environ)
    else:
        merged = _safe_parent_env()
    for key, value in server.env.items():
        merged[key] = value
    return merged or None


def _safe_parent_env() -> dict[str, str]:
    allowlist = [
        "PATH",
        "HOME",
        "USER",
        "USERNAME",
        "TMP",
        "TEMP",
        "TMPDIR",
        "LANG",
        "LC_ALL",
        "SystemRoot",
        "COMSPEC",
        "PATHEXT",
        "WINDIR",
    ]
    env: dict[str, str] = {}
    for key in allowlist:
        value = os.getenv(key)
        if value is not None:
            env[key] = value
    return env


__all__ = [
    "McpClientError",
    "McpDisconnectedError",
    "McpRpcError",
    "McpStdioClient",
    "McpTimeoutError",
    "McpTool",
    "McpToolCallResult",
]
