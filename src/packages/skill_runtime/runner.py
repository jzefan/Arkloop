from __future__ import annotations

from dataclasses import replace
from typing import AsyncIterator, Mapping

from packages.agent_core import (
    AgentRunContext,
    AgentRunner,
    DispatchingToolExecutor,
    RunEvent,
    RunEventEmitter,
    ToolAllowlist,
    ToolExecutor,
    ToolPolicyEnforcer,
    ToolRegistry,
)

from .schema import SkillDefinition, SkillRegistry

_ERROR_CLASS_SKILL_NOT_FOUND = "skill.not_found"
_ERROR_CLASS_SKILL_VERSION_MISMATCH = "skill.version_mismatch"
_ERROR_CLASS_SKILL_INVALID_ID = "skill.invalid_id"


class SkillRunner(AgentRunner):
    def __init__(
        self,
        *,
        base_runner: AgentRunner,
        registry: SkillRegistry,
        tool_registry: ToolRegistry,
        tool_executors: Mapping[str, ToolExecutor],
        base_tool_allowlist_names: frozenset[str],
    ) -> None:
        self._base_runner = base_runner
        self._registry = registry
        self._tool_registry = tool_registry
        self._tool_executors = dict(tool_executors)
        self._base_tool_allowlist_names = frozenset(base_tool_allowlist_names)

    async def run(self, *, context: AgentRunContext) -> AsyncIterator[RunEvent]:
        raw_skill_id = context.input_json.get("skill_id")
        if raw_skill_id is None:
            async for event in self._base_runner.run(context=context):
                yield event
            return

        if not isinstance(raw_skill_id, str) or not raw_skill_id.strip():
            yield _emit_run_failed(
                context=context,
                error_class=_ERROR_CLASS_SKILL_INVALID_ID,
                message="skill_id is invalid",
            )
            return

        try:
            skill_id, requested_version = _parse_skill_ref(raw_skill_id.strip())
        except ValueError:
            yield _emit_run_failed(
                context=context,
                error_class=_ERROR_CLASS_SKILL_INVALID_ID,
                message="skill_id is invalid",
            )
            return
        definition = self._registry.get(skill_id)
        if definition is None:
            yield _emit_run_failed(
                context=context,
                error_class=_ERROR_CLASS_SKILL_NOT_FOUND,
                message="skill not found",
                details={"skill_id": skill_id},
            )
            return

        if requested_version is not None and requested_version != definition.version:
            yield _emit_run_failed(
                context=context,
                error_class=_ERROR_CLASS_SKILL_VERSION_MISMATCH,
                message="skill version mismatch",
                details={
                    "skill_id": skill_id,
                    "requested_version": requested_version,
                    "available_version": definition.version,
                },
            )
            return

        injected_context = self._inject_skill_context(context=context, definition=definition)
        async for event in self._base_runner.run(context=injected_context):
            yield event

    def _inject_skill_context(self, *, context: AgentRunContext, definition: SkillDefinition) -> AgentRunContext:
        effective_allowlist = self._effective_allowlist(definition=definition)
        tool_executor = _build_per_run_executor(
            tool_registry=self._tool_registry,
            tool_executors=self._tool_executors,
            allowlist_names=effective_allowlist,
        )

        budgets = definition.budgets
        overrides: dict[str, object] = {
            "system_prompt": definition.prompt_md,
            "tool_allowlist": tuple(sorted(effective_allowlist)),
            "tool_executor": tool_executor,
        }
        if budgets.max_iterations is not None:
            overrides["max_iterations"] = budgets.max_iterations
        if budgets.max_output_tokens is not None:
            overrides["max_output_tokens"] = budgets.max_output_tokens
        if budgets.tool_timeout_ms is not None:
            overrides["tool_timeout_ms"] = budgets.tool_timeout_ms
        if budgets.tool_budget:
            overrides["tool_budget"] = dict(budgets.tool_budget)

        return replace(context, **overrides)

    def _effective_allowlist(self, *, definition: SkillDefinition) -> frozenset[str]:
        skill_allowlist = frozenset(definition.tool_allowlist)
        return frozenset(self._base_tool_allowlist_names.intersection(skill_allowlist))


def _build_per_run_executor(
    *,
    tool_registry: ToolRegistry,
    tool_executors: Mapping[str, ToolExecutor],
    allowlist_names: frozenset[str],
) -> ToolExecutor:
    allowlist = ToolAllowlist.from_names(allowlist_names)
    policy_enforcer = ToolPolicyEnforcer(registry=tool_registry, allowlist=allowlist)
    return DispatchingToolExecutor(
        registry=tool_registry,
        policy_enforcer=policy_enforcer,
        executors=tool_executors,
    )


def _parse_skill_ref(value: str) -> tuple[str, str | None]:
    skill_id, sep, version = value.partition("@")
    if not sep:
        return skill_id, None
    if not version:
        raise ValueError("skill_id@version format missing version")
    return skill_id, version


def _emit_run_failed(
    *,
    context: AgentRunContext,
    error_class: str,
    message: str,
    details: Mapping[str, object] | None = None,
) -> RunEvent:
    emitter = RunEventEmitter(run_id=context.run_id, trace_id=context.trace_id)
    payload: dict[str, object] = {"error_class": error_class, "message": message}
    if details:
        payload["details"] = dict(details)
    return emitter.emit(type="run.failed", data_json=payload, error_class=error_class)


__all__ = ["SkillRunner"]
