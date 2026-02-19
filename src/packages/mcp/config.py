from __future__ import annotations

from dataclasses import dataclass, field
import json
import os
from pathlib import Path
from typing import Any, Mapping
from urllib.parse import urlsplit, urlunsplit

_MCP_CONFIG_FILE_ENV = "ARKLOOP_MCP_CONFIG_FILE"

_DEFAULT_CALL_TIMEOUT_MS = 10_000


@dataclass(frozen=True, slots=True)
class McpServerConfig:
    server_id: str
    command: str | None = None
    args: tuple[str, ...] = ()
    cwd: str | None = None
    env: Mapping[str, str] = field(default_factory=dict)
    inherit_parent_env: bool = False
    call_timeout_ms: int = _DEFAULT_CALL_TIMEOUT_MS
    transport: str = "stdio"
    sse_url: str | None = None
    sse_bearer_token_env: str | None = None
    sse_bearer_token: str | None = None

    @classmethod
    def from_json(cls, server_id: str, payload: Mapping[str, Any]) -> "McpServerConfig":
        transport = payload.get("transport") or "stdio"
        if not isinstance(transport, str) or not transport.strip():
            raise ValueError(f"MCP server {server_id!r} transport must be a string")
        transport_value = transport.strip().casefold()

        cleaned_id = server_id.strip()
        if not cleaned_id:
            raise ValueError("MCP server_id must not be empty")

        call_timeout_ms = payload.get("callTimeoutMs") or payload.get("call_timeout_ms")
        if call_timeout_ms is None:
            timeout_value = _DEFAULT_CALL_TIMEOUT_MS
        else:
            if not isinstance(call_timeout_ms, int) or call_timeout_ms <= 0:
                raise ValueError(f"MCP server {server_id!r} callTimeoutMs must be a positive integer")
            timeout_value = int(call_timeout_ms)

        if transport_value == "stdio":
            command = payload.get("command")
            if not isinstance(command, str) or not command.strip():
                raise ValueError(f"MCP server {server_id!r} missing command")

            raw_args = payload.get("args") or []
            if not isinstance(raw_args, list) or not all(isinstance(item, str) for item in raw_args):
                raise ValueError(f"MCP server {server_id!r} args must be a string array")
            args = tuple(item for item in (s.strip() for s in raw_args) if item)

            cwd = payload.get("cwd")
            if cwd is not None and (not isinstance(cwd, str) or not cwd.strip()):
                raise ValueError(f"MCP server {server_id!r} cwd must be a string")
            cwd_value = cwd.strip() if isinstance(cwd, str) else None

            raw_env = payload.get("env") or {}
            if not isinstance(raw_env, Mapping):
                raise ValueError(f"MCP server {server_id!r} env must be an object")
            env: dict[str, str] = {}
            for key, value in raw_env.items():
                if not isinstance(key, str) or not key.strip():
                    raise ValueError(f"MCP server {server_id!r} env key invalid")
                if not isinstance(value, str):
                    raise ValueError(f"MCP server {server_id!r} env[{key!r}] must be a string")
                env[key.strip()] = value

            raw_inherit = payload.get("inheritParentEnv")
            if raw_inherit is None:
                raw_inherit = payload.get("inherit_parent_env")
            if raw_inherit is None:
                inherit_parent_env = False
            elif isinstance(raw_inherit, bool):
                inherit_parent_env = raw_inherit
            else:
                raise ValueError(f"MCP server {server_id!r} inheritParentEnv must be a boolean")

            return cls(
                server_id=cleaned_id,
                command=command.strip(),
                args=args,
                cwd=cwd_value,
                env=env,
                inherit_parent_env=inherit_parent_env,
                call_timeout_ms=timeout_value,
                transport=transport_value,
            )

        if transport_value == "sse":
            raw_url = payload.get("url") or payload.get("sseUrl") or payload.get("sse_url")
            if not isinstance(raw_url, str) or not raw_url.strip():
                raise ValueError(f"MCP server {server_id!r} transport=sse missing url")
            sse_url = _normalize_sse_url(raw_url.strip())

            bearer_token_env = payload.get("bearerTokenEnv") or payload.get("bearer_token_env")
            if bearer_token_env is not None and (not isinstance(bearer_token_env, str) or not bearer_token_env.strip()):
                raise ValueError(f"MCP server {server_id!r} bearerTokenEnv must be a string")
            bearer_token_env_value = bearer_token_env.strip() if isinstance(bearer_token_env, str) else None

            bearer_token = payload.get("bearerToken") or payload.get("bearer_token")
            if bearer_token is not None and (not isinstance(bearer_token, str) or not bearer_token.strip()):
                raise ValueError(f"MCP server {server_id!r} bearerToken must be a string")
            bearer_token_value = bearer_token.strip() if isinstance(bearer_token, str) else None

            return cls(
                server_id=cleaned_id,
                command=None,
                args=(),
                cwd=None,
                env={},
                inherit_parent_env=False,
                call_timeout_ms=timeout_value,
                transport=transport_value,
                sse_url=sse_url,
                sse_bearer_token_env=bearer_token_env_value,
                sse_bearer_token=bearer_token_value,
            )

        raise ValueError(f"MCP server {server_id!r} transport not supported: {transport}")



def _normalize_sse_url(raw: str) -> str:
    parts = urlsplit(raw)
    if not parts.scheme or not parts.netloc:
        raise ValueError("MCP SSE url must be an absolute URL (with scheme and host)")

    path = parts.path.rstrip("/")
    if not path.endswith("/sse"):
        if not path:
            path = "/sse"
        else:
            path = f"{path}/sse"

    return urlunsplit((parts.scheme, parts.netloc, path, parts.query, parts.fragment))


@dataclass(frozen=True, slots=True)
class McpConfig:
    servers: tuple[McpServerConfig, ...] = ()

    @classmethod
    def from_json(cls, payload: Mapping[str, Any]) -> "McpConfig":
        raw_servers = payload.get("mcpServers") or payload.get("mcp_servers") or {}
        if not isinstance(raw_servers, Mapping):
            raise ValueError("mcpServers must be an object")

        servers: list[McpServerConfig] = []
        for server_id, raw_cfg in raw_servers.items():
            if not isinstance(server_id, str):
                raise ValueError("mcpServers key must be a string")
            if not isinstance(raw_cfg, Mapping):
                raise ValueError(f"mcpServers[{server_id!r}] must be an object")
            servers.append(McpServerConfig.from_json(server_id, raw_cfg))

        servers.sort(key=lambda item: item.server_id)
        return cls(servers=tuple(servers))

    @classmethod
    def from_file(cls, path: Path) -> "McpConfig":
        content = path.read_text(encoding="utf-8-sig")
        data = json.loads(content)
        if not isinstance(data, Mapping):
            raise ValueError("MCP config file must be a JSON object")
        return cls.from_json(data)

    @classmethod
    def from_env(cls) -> "McpConfig | None":
        raw = (os.getenv(_MCP_CONFIG_FILE_ENV) or "").strip()
        if not raw:
            return None
        path = Path(raw).expanduser()
        if not path.is_file():
            raise ValueError(f"{_MCP_CONFIG_FILE_ENV} points to a file that does not exist: {raw}")
        return cls.from_file(path)


__all__ = ["McpConfig", "McpServerConfig"]
