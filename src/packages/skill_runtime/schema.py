from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Mapping


@dataclass(frozen=True, slots=True)
class SkillBudgets:
    max_iterations: int | None = None
    max_output_tokens: int | None = None
    tool_timeout_ms: int | None = None
    tool_budget: Mapping[str, Any] = field(default_factory=dict)


@dataclass(frozen=True, slots=True)
class SkillDefinition:
    id: str
    version: str
    title: str
    description: str | None = None
    tool_allowlist: tuple[str, ...] = ()
    budgets: SkillBudgets = field(default_factory=SkillBudgets)
    prompt_md: str = ""


class SkillRegistry:
    def __init__(self) -> None:
        self._by_id: dict[str, SkillDefinition] = {}

    def register(self, definition: SkillDefinition) -> None:
        if definition.id in self._by_id:
            raise ValueError(f"skill.id duplicated: {definition.id}")
        self._by_id[definition.id] = definition

    def get(self, skill_id: str) -> SkillDefinition | None:
        return self._by_id.get(skill_id)

    def list_ids(self) -> list[str]:
        return sorted(self._by_id.keys())


__all__ = [
    "SkillBudgets",
    "SkillDefinition",
    "SkillRegistry",
]

