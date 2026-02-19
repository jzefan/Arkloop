from __future__ import annotations

import asyncio
from dataclasses import dataclass
import json
from typing import Any, AsyncIterator, Mapping
import uuid

from packages.llm_gateway import (
    ERROR_CLASS_INTERNAL_STREAM_ENDED,
    LlmGatewayError,
    LlmGatewayRequest,
    LlmMessage,
    LlmStreamLlmRequest,
    LlmStreamLlmResponseChunk,
    LlmStreamMessageDelta,
    LlmStreamProviderFallback,
    LlmStreamRunCompleted,
    LlmStreamRunFailed,
    LlmStreamToolCall,
    LlmStreamToolResult,
    LlmTextPart,
)
from packages.llm_gateway.gateway import LlmGateway

from .events import RunEvent, RunEventEmitter
from .executor import (
    ERROR_CLASS_TOOL_EXECUTION_FAILED,
    ToolExecutionContext,
    ToolExecutionError,
    ToolExecutionResult,
    ToolExecutor,
)
from .runner import AgentRunContext
from .stub_executor import StubToolExecutor

ERROR_CLASS_AGENT_MAX_ITERATIONS_EXCEEDED = "agent.max_iterations_exceeded"


class AgentLoop:
    def __init__(
        self,
        *,
        gateway: LlmGateway,
        tool_executor: ToolExecutor | None = None,
    ) -> None:
        self._gateway = gateway
        self._tool_executor = tool_executor or StubToolExecutor()

    async def run(
        self,
        *,
        context: AgentRunContext,
        emitter: RunEventEmitter,
        request: LlmGatewayRequest,
    ) -> AsyncIterator[RunEvent]:
        if context.max_iterations <= 0:
            yield _emit_max_iterations_failed(emitter=emitter, max_iterations=context.max_iterations)
            return

        messages = list(request.messages)
        for _ in range(1, context.max_iterations + 1):
            if _cancelled(context):
                yield emitter.emit(type="run.cancelled", data_json={"reason": "cancel_signal"})
                return

            turn_request = _copy_request(request=request, messages=messages)
            turn = await self._run_single_turn(
                context=context,
                emitter=emitter,
                request=turn_request,
            )
            for event in turn.events:
                yield event

            if turn.terminal:
                return
            if turn.cancelled:
                yield emitter.emit(type="run.cancelled", data_json={"reason": "cancel_signal"})
                return

            if turn.assistant_text or turn.tool_calls:
                messages.append(_assistant_message(turn.assistant_text, tool_calls=turn.tool_calls))

            for tool_result in turn.tool_results:
                messages.append(_tool_result_message(tool_result))

            completed_tool_result_ids = {item.tool_call_id for item in turn.tool_results}

            if turn.completed_data_json is None:
                error = _internal_stream_ended_error()
                yield emitter.emit(
                    type="run.failed",
                    data_json=error.to_json(),
                    error_class=error.error_class,
                )
                return

            if not turn.tool_calls:
                yield emitter.emit(type="run.completed", data_json=turn.completed_data_json)
                return

            pending_tool_calls = tuple(
                item for item in turn.tool_calls if item.tool_call_id not in completed_tool_result_ids
            )
            if not pending_tool_calls:
                yield emitter.emit(type="run.completed", data_json=turn.completed_data_json)
                return

            if _cancelled(context):
                yield emitter.emit(type="run.cancelled", data_json={"reason": "cancel_signal"})
                return

            execution_outcomes = await self._execute_tool_calls(
                context=context,
                tool_calls=pending_tool_calls,
            )
            for outcome in execution_outcomes:
                emitted_tool_call = False
                for execution_event in outcome.result.events:
                    if execution_event.type == "tool.call":
                        emitted_tool_call = True
                    yield emitter.emit(
                        type=execution_event.type,
                        data_json=execution_event.data_json,
                        tool_name=execution_event.tool_name,
                        error_class=execution_event.error_class,
                    )

                if not emitted_tool_call:
                    yield emitter.emit(
                        type="tool.call",
                        data_json=outcome.tool_call.to_data_json(),
                        tool_name=outcome.tool_call.tool_name,
                    )

                resolved_tool_call_id = _resolved_tool_call_id(outcome=outcome)
                tool_result = _tool_result_from_execution(
                    tool_call_id=resolved_tool_call_id,
                    tool_name=outcome.tool_call.tool_name,
                    execution_result=outcome.result,
                )
                messages.append(_tool_result_message(tool_result))
                yield emitter.emit(
                    type="tool.result",
                    data_json=tool_result.to_data_json(),
                    tool_name=tool_result.tool_name,
                    error_class=tool_result.error.error_class if tool_result.error is not None else None,
                )

        yield _emit_max_iterations_failed(emitter=emitter, max_iterations=context.max_iterations)

    async def _run_single_turn(
        self,
        *,
        context: AgentRunContext,
        emitter: RunEventEmitter,
        request: LlmGatewayRequest,
    ) -> "_TurnResult":
        stream = self._gateway.stream(request=request)
        close = getattr(stream, "aclose", None)

        events: list[RunEvent] = []
        tool_calls: list[LlmStreamToolCall] = []
        tool_results: list[LlmStreamToolResult] = []
        assistant_chunks: list[str] = []
        completed: LlmStreamRunCompleted | None = None

        try:
            async for item in stream:
                if _cancelled(context):
                    return _TurnResult(events=tuple(events), terminal=False, cancelled=True)

                if isinstance(item, LlmStreamMessageDelta):
                    if item.content_delta:
                        assistant_chunks.append(item.content_delta)
                        events.append(emitter.emit(type="message.delta", data_json=item.to_data_json()))
                    continue

                if isinstance(item, LlmStreamLlmRequest):
                    events.append(emitter.emit(type="llm.request", data_json=item.to_data_json()))
                    continue

                if isinstance(item, LlmStreamLlmResponseChunk):
                    events.append(emitter.emit(type="llm.response.chunk", data_json=item.to_data_json()))
                    continue

                if isinstance(item, LlmStreamProviderFallback):
                    events.append(emitter.emit(type="run.provider_fallback", data_json=item.to_data_json()))
                    continue

                if isinstance(item, LlmStreamToolCall):
                    tool_calls.append(item)
                    continue

                if isinstance(item, LlmStreamToolResult):
                    tool_results.append(item)
                    events.append(
                        emitter.emit(
                            type="tool.result",
                            data_json=item.to_data_json(),
                            tool_name=item.tool_name,
                            error_class=item.error.error_class if item.error is not None else None,
                        )
                    )
                    continue

                if isinstance(item, LlmStreamRunFailed):
                    events.append(
                        emitter.emit(
                            type="run.failed",
                            data_json=item.to_data_json(),
                            error_class=item.error.error_class,
                        )
                    )
                    return _TurnResult(events=tuple(events), terminal=True)

                if isinstance(item, LlmStreamRunCompleted):
                    completed = item
                    break

                raise TypeError(f"unknown LLM gateway event type: {type(item)!r}")
        finally:
            if close is not None:
                try:
                    await close()
                except Exception:
                    pass

        completed_payload = completed.to_data_json() if completed is not None else None
        return _TurnResult(
            events=tuple(events),
            terminal=False,
            tool_calls=tuple(tool_calls),
            tool_results=tuple(tool_results),
            assistant_text="".join(assistant_chunks),
            completed_data_json=completed_payload,
        )

    async def _execute_tool_calls(
        self,
        *,
        context: AgentRunContext,
        tool_calls: tuple[LlmStreamToolCall, ...],
    ) -> tuple["_ToolExecutionOutcome", ...]:
        execution_context = _tool_execution_context(context=context)
        execution_tasks = [
            self._tool_executor.execute(
                tool_name=tool_call.tool_name,
                args=dict(tool_call.arguments_json),
                context=execution_context,
                tool_call_id=tool_call.tool_call_id,
            )
            for tool_call in tool_calls
        ]
        raw_results = await asyncio.gather(*execution_tasks, return_exceptions=True)

        outcomes: list[_ToolExecutionOutcome] = []
        for tool_call, raw_result in zip(tool_calls, raw_results):
            if isinstance(raw_result, ToolExecutionResult):
                result = raw_result
            elif isinstance(raw_result, Exception):
                result = ToolExecutionResult(
                    error=ToolExecutionError(
                        error_class=ERROR_CLASS_TOOL_EXECUTION_FAILED,
                        message="tool execution failed",
                        details={
                            "tool_name": tool_call.tool_name,
                            "tool_call_id": tool_call.tool_call_id,
                            "exception_type": type(raw_result).__name__,
                        },
                    )
                )
            else:
                result = ToolExecutionResult(
                    error=ToolExecutionError(
                        error_class=ERROR_CLASS_TOOL_EXECUTION_FAILED,
                        message="tool execution returned unknown result",
                        details={
                            "tool_name": tool_call.tool_name,
                            "tool_call_id": tool_call.tool_call_id,
                        },
                    )
                )
            outcomes.append(_ToolExecutionOutcome(tool_call=tool_call, result=result))
        return tuple(outcomes)


@dataclass(frozen=True, slots=True)
class _ToolExecutionOutcome:
    tool_call: LlmStreamToolCall
    result: ToolExecutionResult


@dataclass(frozen=True, slots=True)
class _TurnResult:
    events: tuple[RunEvent, ...]
    terminal: bool
    cancelled: bool = False
    tool_calls: tuple[LlmStreamToolCall, ...] = ()
    tool_results: tuple[LlmStreamToolResult, ...] = ()
    assistant_text: str = ""
    completed_data_json: Mapping[str, Any] | None = None


def _copy_request(*, request: LlmGatewayRequest, messages: list[LlmMessage]) -> LlmGatewayRequest:
    return LlmGatewayRequest(
        model=request.model,
        messages=list(messages),
        temperature=request.temperature,
        max_output_tokens=request.max_output_tokens,
        tools=list(request.tools),
        metadata=dict(request.metadata),
    )


def _assistant_message(content: str, *, tool_calls: tuple[LlmStreamToolCall, ...] = ()) -> LlmMessage:
    parts = [LlmTextPart(text=content)] if content else []
    return LlmMessage(role="assistant", content=parts, tool_calls=list(tool_calls))


def _tool_result_from_execution(
    *,
    tool_call_id: str,
    tool_name: str,
    execution_result: ToolExecutionResult,
) -> LlmStreamToolResult:
    error = None
    if execution_result.error is not None:
        error = LlmGatewayError(
            error_class=execution_result.error.error_class,
            message=execution_result.error.message,
            details=dict(execution_result.error.details),
        )
    return LlmStreamToolResult(
        tool_call_id=tool_call_id,
        tool_name=tool_name,
        result_json=(
            dict(execution_result.result_json)
            if execution_result.result_json is not None
            else None
        ),
        error=error,
    )


def _resolved_tool_call_id(*, outcome: _ToolExecutionOutcome) -> str:
    for event in outcome.result.events:
        if event.type != "tool.call":
            continue
        tool_call_id = event.data_json.get("tool_call_id")
        if isinstance(tool_call_id, str) and tool_call_id.strip():
            return tool_call_id
    return outcome.tool_call.tool_call_id


def _tool_result_message(tool_result: LlmStreamToolResult) -> LlmMessage:
    payload: dict[str, Any] = {
        "tool_call_id": tool_result.tool_call_id,
        "tool_name": tool_result.tool_name,
    }
    if tool_result.result_json is not None:
        payload["result"] = dict(tool_result.result_json)
    if tool_result.error is not None:
        payload["error"] = tool_result.error.to_json()
    content = json.dumps(payload, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    return LlmMessage(role="tool", content=[LlmTextPart(text=content)])


def _tool_execution_context(*, context: AgentRunContext) -> ToolExecutionContext:
    return ToolExecutionContext(
        run_id=context.run_id,
        trace_id=context.trace_id,
        org_id=_optional_uuid(context.input_json.get("org_id")),
        timeout_ms=context.tool_timeout_ms,
        budget=dict(context.tool_budget),
    )


def _optional_uuid(value: object) -> uuid.UUID | None:
    if not isinstance(value, str):
        return None
    cleaned = value.strip()
    if not cleaned:
        return None
    try:
        return uuid.UUID(cleaned)
    except ValueError:
        return None


def _cancelled(context: AgentRunContext) -> bool:
    if context.cancel_signal is None:
        return False
    return bool(context.cancel_signal())


def _internal_stream_ended_error() -> LlmGatewayError:
    return LlmGatewayError(
        error_class=ERROR_CLASS_INTERNAL_STREAM_ENDED,
        message="upstream stream ended prematurely without terminal state",
    )


def _emit_max_iterations_failed(*, emitter: RunEventEmitter, max_iterations: int) -> RunEvent:
    error = LlmGatewayError(
        error_class=ERROR_CLASS_AGENT_MAX_ITERATIONS_EXCEEDED,
        message="agent loop reached max iterations",
        details={"max_iterations": max_iterations},
    )
    return emitter.emit(type="run.failed", data_json=error.to_json(), error_class=error.error_class)


__all__ = [
    "AgentLoop",
    "ERROR_CLASS_AGENT_MAX_ITERATIONS_EXCEEDED",
    "StubToolExecutor",
]
