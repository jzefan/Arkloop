from __future__ import annotations

import time
from typing import Any, Mapping

from packages.agent_core.executor import (
    ToolExecutionContext,
    ToolExecutionError,
    ToolExecutionResult,
)

from .client import (
    McpClientError,
    McpDisconnectedError,
    McpRpcError,
    McpStdioClient,
    McpTimeoutError,
)
from .config import McpServerConfig

ERROR_CLASS_MCP_TIMEOUT = "mcp.timeout"
ERROR_CLASS_MCP_DISCONNECTED = "mcp.disconnected"
ERROR_CLASS_MCP_RPC_ERROR = "mcp.rpc_error"
ERROR_CLASS_MCP_PROTOCOL_ERROR = "mcp.protocol_error"
ERROR_CLASS_MCP_TOOL_ERROR = "mcp.tool_error"


class McpToolExecutor:
    def __init__(
        self,
        *,
        server: McpServerConfig,
        remote_tool_name_by_tool_name: Mapping[str, str],
    ) -> None:
        self._server = server
        self._remote_tool_name_by_tool_name = dict(remote_tool_name_by_tool_name)

    async def execute(
        self,
        *,
        tool_name: str,
        args: dict[str, Any],
        context: ToolExecutionContext,
        tool_call_id: str | None = None,
    ) -> ToolExecutionResult:
        _ = (context, tool_call_id)
        started = time.monotonic()

        remote_name = self._remote_tool_name_by_tool_name.get(tool_name)
        if remote_name is None:
            return ToolExecutionResult(
                error=ToolExecutionError(
                    error_class=ERROR_CLASS_MCP_PROTOCOL_ERROR,
                    message="MCP 工具未注册",
                    details={"tool_name": tool_name, "server_id": self._server.server_id},
                ),
                duration_ms=_duration_ms(started),
            )

        timeout_ms = context.timeout_ms or self._server.call_timeout_ms

        try:
            async with McpStdioClient(server=self._server) as client:
                result = await client.call_tool(name=remote_name, arguments=args, timeout_ms=timeout_ms)
        except McpTimeoutError as exc:
            return ToolExecutionResult(
                error=ToolExecutionError(
                    error_class=ERROR_CLASS_MCP_TIMEOUT,
                    message=str(exc),
                    details={"tool_name": tool_name, "server_id": self._server.server_id},
                ),
                duration_ms=_duration_ms(started),
            )
        except McpDisconnectedError as exc:
            return ToolExecutionResult(
                error=ToolExecutionError(
                    error_class=ERROR_CLASS_MCP_DISCONNECTED,
                    message=str(exc),
                    details={"tool_name": tool_name, "server_id": self._server.server_id},
                ),
                duration_ms=_duration_ms(started),
            )
        except McpRpcError as exc:
            return ToolExecutionResult(
                error=ToolExecutionError(
                    error_class=ERROR_CLASS_MCP_RPC_ERROR,
                    message=str(exc),
                    details={
                        "tool_name": tool_name,
                        "server_id": self._server.server_id,
                        "code": exc.code,
                        "data": exc.data,
                    },
                ),
                duration_ms=_duration_ms(started),
            )
        except McpClientError as exc:
            return ToolExecutionResult(
                error=ToolExecutionError(
                    error_class=ERROR_CLASS_MCP_PROTOCOL_ERROR,
                    message=str(exc),
                    details={"tool_name": tool_name, "server_id": self._server.server_id},
                ),
                duration_ms=_duration_ms(started),
            )

        if result.is_error:
            return ToolExecutionResult(
                error=ToolExecutionError(
                    error_class=ERROR_CLASS_MCP_TOOL_ERROR,
                    message="MCP 工具返回错误",
                    details={
                        "tool_name": tool_name,
                        "server_id": self._server.server_id,
                        "content": result.content,
                    },
                ),
                duration_ms=_duration_ms(started),
            )

        return ToolExecutionResult(
            result_json={"content": result.content},
            duration_ms=_duration_ms(started),
        )


def _duration_ms(started: float) -> int:
    elapsed = time.monotonic() - started
    millis = int(elapsed * 1000)
    return millis if millis >= 0 else 0


__all__ = [
    "ERROR_CLASS_MCP_DISCONNECTED",
    "ERROR_CLASS_MCP_PROTOCOL_ERROR",
    "ERROR_CLASS_MCP_RPC_ERROR",
    "ERROR_CLASS_MCP_TIMEOUT",
    "ERROR_CLASS_MCP_TOOL_ERROR",
    "McpToolExecutor",
]
