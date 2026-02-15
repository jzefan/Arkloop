from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any, AsyncIterator, Callable, Mapping, Protocol
import uuid

from .events import RunEvent
from .executor import ToolExecutor

if TYPE_CHECKING:
    from packages.llm_gateway import ToolSpec as LlmToolSpec


CancelSignal = Callable[[], bool]


@dataclass(frozen=True, slots=True)
class AgentRunContext:
    run_id: uuid.UUID
    trace_id: str | None = None
    input_json: Mapping[str, Any] = field(default_factory=dict)
    max_iterations: int = 10
    system_prompt: str | None = None
    tool_allowlist: tuple[str, ...] | None = None
    max_output_tokens: int | None = None
    tool_timeout_ms: int | None = None
    tool_budget: Mapping[str, Any] = field(default_factory=dict)
    tool_executor: ToolExecutor | None = None
    tool_specs: tuple["LlmToolSpec", ...] = field(default_factory=tuple)
    cancel_signal: CancelSignal | None = None


class AgentRunner(Protocol):
    async def run(self, *, context: AgentRunContext) -> AsyncIterator[RunEvent]: ...


__all__ = ["AgentRunContext", "AgentRunner", "CancelSignal"]
