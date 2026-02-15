from __future__ import annotations

import uuid

import anyio

from packages.agent_core import (
    AgentRunContext,
    RunEvent,
    RunEventEmitter,
    ToolExecutionContext,
    ToolRegistry,
)
from packages.agent_core.builtin_tools import builtin_agent_tool_specs, create_builtin_tool_executors
from packages.skill_runtime import SkillRunner
from packages.skill_runtime.schema import SkillBudgets, SkillDefinition, SkillRegistry


class _CapturingRunner:
    def __init__(self) -> None:
        self.contexts: list[AgentRunContext] = []

    async def run(self, *, context: AgentRunContext):
        self.contexts.append(context)
        emitter = RunEventEmitter(run_id=context.run_id, trace_id=context.trace_id)
        yield emitter.emit(type="run.completed", data_json={})


def test_skill_runner_injects_budgets_and_enforces_allowlist() -> None:
    tool_registry = ToolRegistry(specs=builtin_agent_tool_specs())
    executors = dict(create_builtin_tool_executors())

    skill_registry = SkillRegistry()
    skill_registry.register(
        SkillDefinition(
            id="demo",
            version="1",
            title="Demo",
            tool_allowlist=("noop",),
            budgets=SkillBudgets(
                max_iterations=3,
                max_output_tokens=64,
                tool_timeout_ms=123,
                tool_budget={"max_cost_micros": 1},
            ),
            prompt_md="PROMPT",
        )
    )

    base_runner = _CapturingRunner()
    runner = SkillRunner(
        base_runner=base_runner,
        registry=skill_registry,
        tool_registry=tool_registry,
        tool_executors=executors,
        base_tool_allowlist_names=frozenset({"echo", "noop"}),
    )

    context = AgentRunContext(run_id=uuid.uuid4(), trace_id="t" * 32, input_json={"skill_id": "demo@1"})

    async def _collect() -> list[RunEvent]:
        events: list[RunEvent] = []
        async for event in runner.run(context=context):
            events.append(event)
        return events

    events = anyio.run(_collect)
    assert [event.type for event in events] == ["run.completed"]
    assert len(base_runner.contexts) == 1

    injected = base_runner.contexts[0]
    assert injected.system_prompt == "PROMPT"
    assert injected.tool_allowlist == ("noop",)
    assert injected.max_iterations == 3
    assert injected.max_output_tokens == 64
    assert injected.tool_timeout_ms == 123
    assert injected.tool_budget == {"max_cost_micros": 1}
    tool_executor = injected.tool_executor
    assert tool_executor is not None

    async def _execute_denied() -> None:
        execution_context = ToolExecutionContext(run_id=injected.run_id, trace_id=injected.trace_id)
        result = await tool_executor.execute(
            tool_name="echo",
            args={"text": "hi"},
            context=execution_context,
            tool_call_id="call_1",
        )
        assert result.error is not None
        assert result.error.error_class == "policy.denied"
        assert any(event.type == "policy.denied" for event in result.events)

    anyio.run(_execute_denied)


def test_skill_runner_returns_failed_when_skill_missing() -> None:
    tool_registry = ToolRegistry(specs=builtin_agent_tool_specs())
    executors = dict(create_builtin_tool_executors())
    skill_registry = SkillRegistry()
    base_runner = _CapturingRunner()
    runner = SkillRunner(
        base_runner=base_runner,
        registry=skill_registry,
        tool_registry=tool_registry,
        tool_executors=executors,
        base_tool_allowlist_names=frozenset({"echo"}),
    )

    context = AgentRunContext(
        run_id=uuid.uuid4(),
        trace_id="t" * 32,
        input_json={"skill_id": "missing"},
    )

    async def _collect() -> list[RunEvent]:
        events: list[RunEvent] = []
        async for event in runner.run(context=context):
            events.append(event)
        return events

    events = anyio.run(_collect)
    assert len(events) == 1
    assert events[0].type == "run.failed"
    assert events[0].error_class == "skill.not_found"
    assert base_runner.contexts == []
