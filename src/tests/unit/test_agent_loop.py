from __future__ import annotations

import json
import uuid

import anyio

from packages.agent_core import (
    POLICY_DENIED_CODE,
    AgentRunContext,
    DispatchingToolExecutor,
    RunEventEmitter,
    ToolAllowlist,
    ToolExecutionContext,
    ToolExecutionResult,
    ToolPolicyEnforcer,
    ToolRegistry,
    ToolSpec as AgentToolSpec,
)
from packages.agent_core.loop import ERROR_CLASS_AGENT_MAX_ITERATIONS_EXCEEDED, AgentLoop
from packages.llm_gateway import (
    LlmGatewayRequest,
    LlmMessage,
    LlmStreamMessageDelta,
    LlmStreamRunCompleted,
    LlmStreamToolCall,
    LlmStreamToolResult,
    LlmTextPart,
    ToolSpec as LlmToolSpec,
)


class _ScriptedGateway:
    def __init__(self, *, turns: list[list[object]]) -> None:
        self._turns = turns
        self.requests: list[LlmGatewayRequest] = []

    async def stream(self, *, request: LlmGatewayRequest):
        self.requests.append(request)
        turn_index = len(self.requests) - 1
        items = self._turns[turn_index] if turn_index < len(self._turns) else []
        for item in items:
            yield item


class _RecordingBoundExecutor:
    def __init__(self, *, cancel_state: dict[str, bool] | None = None) -> None:
        self.calls: list[tuple[str, dict[str, object], str | None]] = []
        self._cancel_state = cancel_state

    async def execute(
        self,
        *,
        tool_name: str,
        args: dict[str, object],
        context: ToolExecutionContext,
        tool_call_id: str | None = None,
    ) -> ToolExecutionResult:
        _ = context
        self.calls.append((tool_name, dict(args), tool_call_id))
        if self._cancel_state is not None:
            self._cancel_state["cancelled"] = True
        return ToolExecutionResult(
            result_json={
                "from_bound_executor": True,
                "tool_name": tool_name,
                "tool_call_id": tool_call_id,
                "echo": dict(args),
            }
        )


def _base_request() -> LlmGatewayRequest:
    return LlmGatewayRequest(
        model="stub-model",
        messages=[LlmMessage(role="user", content=[LlmTextPart(text="hi")])],
        tools=[LlmToolSpec(name="echo", description="echo tool", json_schema={"type": "object"})],
    )


def _collect_events(*, loop: AgentLoop, context) -> list:
    emitter = RunEventEmitter(run_id=context.run_id, trace_id=context.trace_id)

    async def _collect():
        events = []
        async for event in loop.run(context=context, emitter=emitter, request=_base_request()):
            events.append(event)
        return events

    return anyio.run(_collect)


def test_agent_loop_supports_multi_turn_with_parallel_tool_calls() -> None:
    gateway = _ScriptedGateway(
        turns=[
            [
                LlmStreamToolCall(
                    tool_call_id="call_1",
                    tool_name="echo",
                    arguments_json={"query": "ping"},
                ),
                LlmStreamToolCall(
                    tool_call_id="call_2",
                    tool_name="echo",
                    arguments_json={"query": "pong"},
                ),
                LlmStreamRunCompleted(),
            ],
            [
                LlmStreamMessageDelta(content_delta="done", role="assistant"),
                LlmStreamRunCompleted(),
            ],
        ]
    )
    bound_executor = _RecordingBoundExecutor()
    registry = ToolRegistry(
        specs=[AgentToolSpec(name="echo", version="1", description="回显", risk_level="low")]
    )
    allowlist = ToolAllowlist.from_names(["echo"])
    dispatcher = DispatchingToolExecutor(
        registry=registry,
        policy_enforcer=ToolPolicyEnforcer(registry=registry, allowlist=allowlist),
        executors={"echo": bound_executor},
    )
    loop = AgentLoop(gateway=gateway, tool_executor=dispatcher)
    context = AgentRunContext(
        run_id=uuid.uuid4(),
        trace_id="a" * 32,
    )

    events = _collect_events(loop=loop, context=context)

    assert [event.type for event in events] == [
        "tool.call",
        "tool.result",
        "tool.call",
        "tool.result",
        "message.delta",
        "run.completed",
    ]
    assert bound_executor.calls == [
        ("echo", {"query": "ping"}, "call_1"),
        ("echo", {"query": "pong"}, "call_2"),
    ]
    assert len(gateway.requests) == 2
    assistant_tool_calls = [
        message
        for message in gateway.requests[1].messages
        if message.role == "assistant" and message.tool_calls
    ]
    assert len(assistant_tool_calls) == 1
    assert [tool_call.tool_call_id for tool_call in assistant_tool_calls[0].tool_calls] == [
        "call_1",
        "call_2",
    ]
    tool_messages = [message for message in gateway.requests[1].messages if message.role == "tool"]
    assert len(tool_messages) == 2
    payloads = [json.loads(message.content[0].text) for message in tool_messages]
    assert [payload["tool_call_id"] for payload in payloads] == ["call_1", "call_2"]


def test_agent_loop_policy_denied_is_sent_back_to_llm_and_keeps_tool_call_id() -> None:
    gateway = _ScriptedGateway(
        turns=[
            [
                LlmStreamToolCall(
                    tool_call_id="deny_1",
                    tool_name="shell",
                    arguments_json={"cmd": "echo hi"},
                ),
                LlmStreamRunCompleted(),
            ],
            [
                LlmStreamMessageDelta(content_delta="继续", role="assistant"),
                LlmStreamRunCompleted(),
            ],
        ]
    )
    bound_executor = _RecordingBoundExecutor()
    registry = ToolRegistry(
        specs=[AgentToolSpec(name="shell", version="1", description="执行命令", risk_level="high")]
    )
    dispatcher = DispatchingToolExecutor(
        registry=registry,
        policy_enforcer=ToolPolicyEnforcer(
            registry=registry,
            allowlist=ToolAllowlist.from_names([]),
        ),
        executors={"shell": bound_executor},
    )
    loop = AgentLoop(gateway=gateway, tool_executor=dispatcher)
    context = AgentRunContext(
        run_id=uuid.uuid4(),
        trace_id="b" * 32,
    )

    events = _collect_events(loop=loop, context=context)

    assert [event.type for event in events] == [
        "tool.call",
        "policy.denied",
        "tool.result",
        "message.delta",
        "run.completed",
    ]
    assert bound_executor.calls == []
    tool_call_id = events[0].data_json["tool_call_id"]
    assert tool_call_id == "deny_1"
    assert events[1].data_json["tool_call_id"] == tool_call_id
    assert events[2].data_json["tool_call_id"] == tool_call_id
    assert events[2].error_class == POLICY_DENIED_CODE

    tool_message = gateway.requests[1].messages[-1]
    assert tool_message.role == "tool"
    payload = json.loads(tool_message.content[0].text)
    assert payload["tool_call_id"] == "deny_1"
    assert payload["error"]["error_class"] == POLICY_DENIED_CODE

    assistant_tool_calls = [
        message
        for message in gateway.requests[1].messages
        if message.role == "assistant" and message.tool_calls
    ]
    assert len(assistant_tool_calls) == 1
    assert [tool_call.tool_call_id for tool_call in assistant_tool_calls[0].tool_calls] == [
        "deny_1",
    ]


def test_agent_loop_rewrites_blank_tool_call_id_consistently() -> None:
    gateway = _ScriptedGateway(
        turns=[
            [
                LlmStreamToolCall(
                    tool_call_id="   ",
                    tool_name="shell",
                    arguments_json={"cmd": "echo hi"},
                ),
                LlmStreamRunCompleted(),
            ],
            [
                LlmStreamMessageDelta(content_delta="继续", role="assistant"),
                LlmStreamRunCompleted(),
            ],
        ]
    )
    registry = ToolRegistry(
        specs=[AgentToolSpec(name="shell", version="1", description="执行命令", risk_level="high")]
    )
    dispatcher = DispatchingToolExecutor(
        registry=registry,
        policy_enforcer=ToolPolicyEnforcer(
            registry=registry,
            allowlist=ToolAllowlist.from_names([]),
        ),
        executors={"shell": _RecordingBoundExecutor()},
    )
    loop = AgentLoop(gateway=gateway, tool_executor=dispatcher)
    context = AgentRunContext(
        run_id=uuid.uuid4(),
        trace_id="c" * 32,
    )

    events = _collect_events(loop=loop, context=context)
    assert [event.type for event in events] == [
        "tool.call",
        "policy.denied",
        "tool.result",
        "message.delta",
        "run.completed",
    ]
    tool_call_id = events[0].data_json["tool_call_id"]
    assert tool_call_id
    uuid.UUID(tool_call_id)
    assert events[1].data_json["tool_call_id"] == tool_call_id
    assert events[2].data_json["tool_call_id"] == tool_call_id

    tool_message = gateway.requests[1].messages[-1]
    payload = json.loads(tool_message.content[0].text)
    assert payload["tool_call_id"] == tool_call_id


def test_agent_loop_does_not_reexecute_when_gateway_already_returned_tool_result() -> None:
    gateway = _ScriptedGateway(
        turns=[
            [
                LlmStreamToolCall(
                    tool_call_id="call_1",
                    tool_name="echo",
                    arguments_json={"query": "from-gateway"},
                ),
                LlmStreamToolResult(
                    tool_call_id="call_1",
                    tool_name="echo",
                    result_json={"from_gateway": True},
                ),
                LlmStreamRunCompleted(),
            ],
        ]
    )
    bound_executor = _RecordingBoundExecutor()
    registry = ToolRegistry(
        specs=[AgentToolSpec(name="echo", version="1", description="回显", risk_level="low")]
    )
    dispatcher = DispatchingToolExecutor(
        registry=registry,
        policy_enforcer=ToolPolicyEnforcer(
            registry=registry,
            allowlist=ToolAllowlist.from_names(["echo"]),
        ),
        executors={"echo": bound_executor},
    )
    loop = AgentLoop(gateway=gateway, tool_executor=dispatcher)
    context = AgentRunContext(
        run_id=uuid.uuid4(),
        trace_id="d" * 32,
        max_iterations=3,
    )

    events = _collect_events(loop=loop, context=context)

    assert [event.type for event in events] == ["tool.result", "run.completed"]
    assert bound_executor.calls == []
    assert events[0].data_json["result"] == {"from_gateway": True}
    assert len(gateway.requests) == 1


def test_agent_loop_emits_failed_when_max_iterations_exceeded() -> None:
    gateway = _ScriptedGateway(
        turns=[
            [
                LlmStreamToolCall(
                    tool_call_id="tool_1",
                    tool_name="echo",
                    arguments_json={"step": 1},
                ),
                LlmStreamRunCompleted(),
            ],
            [
                LlmStreamToolCall(
                    tool_call_id="tool_2",
                    tool_name="echo",
                    arguments_json={"step": 2},
                ),
                LlmStreamRunCompleted(),
            ],
        ]
    )
    loop = AgentLoop(gateway=gateway)
    context = AgentRunContext(
        run_id=uuid.uuid4(),
        trace_id="e" * 32,
        max_iterations=2,
    )

    events = _collect_events(loop=loop, context=context)

    assert [event.type for event in events] == [
        "tool.call",
        "tool.result",
        "tool.call",
        "tool.result",
        "run.failed",
    ]
    assert events[-1].error_class == ERROR_CLASS_AGENT_MAX_ITERATIONS_EXCEEDED
    assert events[-1].data_json["details"]["max_iterations"] == 2


def test_agent_loop_stops_when_cancel_signal_triggered() -> None:
    cancel_state = {"cancelled": False}
    gateway = _ScriptedGateway(
        turns=[
            [
                LlmStreamToolCall(
                    tool_call_id="tool_1",
                    tool_name="echo",
                    arguments_json={"query": "stop"},
                ),
                LlmStreamRunCompleted(),
            ],
            [
                LlmStreamMessageDelta(content_delta="should-not-run", role="assistant"),
                LlmStreamRunCompleted(),
            ],
        ]
    )
    cancel_executor = _RecordingBoundExecutor(cancel_state=cancel_state)
    loop = AgentLoop(gateway=gateway, tool_executor=cancel_executor)
    context = AgentRunContext(
        run_id=uuid.uuid4(),
        trace_id="f" * 32,
        cancel_signal=lambda: cancel_state["cancelled"],
    )

    events = _collect_events(loop=loop, context=context)

    assert [event.type for event in events] == ["tool.call", "tool.result", "run.cancelled"]
    assert events[-1].data_json["reason"] == "cancel_signal"
    assert len(gateway.requests) == 1
