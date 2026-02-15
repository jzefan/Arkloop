from __future__ import annotations

from dataclasses import dataclass, field
import json
import os
from pathlib import Path
from typing import Any, Mapping

_MCP_CONFIG_FILE_ENV = "ARKLOOP_MCP_CONFIG_FILE"

_DEFAULT_CALL_TIMEOUT_MS = 10_000


@dataclass(frozen=True, slots=True)
class McpServerConfig:
    server_id: str
    command: str
    args: tuple[str, ...] = ()
    cwd: str | None = None
    env: Mapping[str, str] = field(default_factory=dict)
    inherit_parent_env: bool = False
    call_timeout_ms: int = _DEFAULT_CALL_TIMEOUT_MS
    transport: str = "stdio"

    @classmethod
    def from_json(cls, server_id: str, payload: Mapping[str, Any]) -> "McpServerConfig":
        command = payload.get("command")
        if not isinstance(command, str) or not command.strip():
            raise ValueError(f"MCP server {server_id!r} 缺少 command")

        raw_args = payload.get("args") or []
        if not isinstance(raw_args, list) or not all(isinstance(item, str) for item in raw_args):
            raise ValueError(f"MCP server {server_id!r} args 必须为字符串数组")
        args = tuple(item for item in (s.strip() for s in raw_args) if item)

        cwd = payload.get("cwd")
        if cwd is not None and (not isinstance(cwd, str) or not cwd.strip()):
            raise ValueError(f"MCP server {server_id!r} cwd 必须为字符串")
        cwd_value = cwd.strip() if isinstance(cwd, str) else None

        raw_env = payload.get("env") or {}
        if not isinstance(raw_env, Mapping):
            raise ValueError(f"MCP server {server_id!r} env 必须为对象")
        env: dict[str, str] = {}
        for key, value in raw_env.items():
            if not isinstance(key, str) or not key.strip():
                raise ValueError(f"MCP server {server_id!r} env key 非法")
            if not isinstance(value, str):
                raise ValueError(f"MCP server {server_id!r} env[{key!r}] 必须为字符串")
            env[key.strip()] = value

        raw_inherit = payload.get("inheritParentEnv")
        if raw_inherit is None:
            raw_inherit = payload.get("inherit_parent_env")
        if raw_inherit is None:
            inherit_parent_env = False
        elif isinstance(raw_inherit, bool):
            inherit_parent_env = raw_inherit
        else:
            raise ValueError(f"MCP server {server_id!r} inheritParentEnv 必须为布尔值")

        call_timeout_ms = payload.get("callTimeoutMs") or payload.get("call_timeout_ms")
        if call_timeout_ms is None:
            timeout_value = _DEFAULT_CALL_TIMEOUT_MS
        else:
            if not isinstance(call_timeout_ms, int) or call_timeout_ms <= 0:
                raise ValueError(f"MCP server {server_id!r} callTimeoutMs 必须为正整数")
            timeout_value = int(call_timeout_ms)

        transport = payload.get("transport") or "stdio"
        if not isinstance(transport, str) or not transport.strip():
            raise ValueError(f"MCP server {server_id!r} transport 必须为字符串")
        transport_value = transport.strip().casefold()
        if transport_value != "stdio":
            raise ValueError(f"MCP server {server_id!r} transport 暂不支持: {transport}")

        cleaned_id = server_id.strip()
        if not cleaned_id:
            raise ValueError("MCP server_id 不能为空")

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


@dataclass(frozen=True, slots=True)
class McpConfig:
    servers: tuple[McpServerConfig, ...] = ()

    @classmethod
    def from_json(cls, payload: Mapping[str, Any]) -> "McpConfig":
        raw_servers = payload.get("mcpServers") or payload.get("mcp_servers") or {}
        if not isinstance(raw_servers, Mapping):
            raise ValueError("mcpServers 必须为对象")

        servers: list[McpServerConfig] = []
        for server_id, raw_cfg in raw_servers.items():
            if not isinstance(server_id, str):
                raise ValueError("mcpServers key 必须为字符串")
            if not isinstance(raw_cfg, Mapping):
                raise ValueError(f"mcpServers[{server_id!r}] 必须为对象")
            servers.append(McpServerConfig.from_json(server_id, raw_cfg))

        servers.sort(key=lambda item: item.server_id)
        return cls(servers=tuple(servers))

    @classmethod
    def from_file(cls, path: Path) -> "McpConfig":
        content = path.read_text(encoding="utf-8-sig")
        data = json.loads(content)
        if not isinstance(data, Mapping):
            raise ValueError("MCP 配置文件必须为 JSON 对象")
        return cls.from_json(data)

    @classmethod
    def from_env(cls) -> "McpConfig | None":
        raw = (os.getenv(_MCP_CONFIG_FILE_ENV) or "").strip()
        if not raw:
            return None
        path = Path(raw).expanduser()
        if not path.is_file():
            raise ValueError(f"{_MCP_CONFIG_FILE_ENV} 指向的文件不存在: {raw}")
        return cls.from_file(path)


__all__ = ["McpConfig", "McpServerConfig"]
