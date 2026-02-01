from __future__ import annotations

from dataclasses import dataclass
import hashlib
import json
from typing import Any, Iterable, Literal, Mapping
import uuid

from .events import RunEvent, RunEventEmitter

RiskLevel = Literal["low", "medium", "high"]

POLICY_DENIED_CODE = "policy.denied"
DENY_REASON_TOOL_NOT_IN_ALLOWLIST = "tool.not_in_allowlist"
DENY_REASON_TOOL_ARGS_INVALID = "tool.args_invalid"
DENY_REASON_TOOL_UNKNOWN = "tool.unknown"


def _stable_json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def _sha256_hex(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def sha256_json(value: Any) -> str:
    return _sha256_hex(_stable_json(value))


@dataclass(frozen=True, slots=True)
class ToolSpec:
    name: str
    version: str
    description: str
    risk_level: RiskLevel = "low"
    required_scopes: tuple[str, ...] = ()
    side_effects: bool = False

    def to_tool_call_json(self) -> dict[str, Any]:
        return {
            "tool_name": self.name,
            "tool_version": self.version,
            "risk_level": self.risk_level,
            "required_scopes": list(self.required_scopes),
            "side_effects": self.side_effects,
        }


class ToolRegistry:
    def __init__(self, specs: Iterable[ToolSpec] = ()) -> None:
        self._spec_by_name: dict[str, ToolSpec] = {}
        for spec in specs:
            self.register(spec)

    def register(self, spec: ToolSpec) -> None:
        existing = self._spec_by_name.get(spec.name)
        if existing is not None:
            raise ValueError(f"Tool 已注册：{spec.name}")
        self._spec_by_name[spec.name] = spec

    def get(self, tool_name: str) -> ToolSpec | None:
        return self._spec_by_name.get(tool_name)

    def list_names(self) -> list[str]:
        return sorted(self._spec_by_name.keys())


@dataclass(frozen=True, slots=True)
class ToolAllowlist:
    allowed: frozenset[str]

    @classmethod
    def from_names(cls, names: Iterable[str]) -> "ToolAllowlist":
        return cls(allowed=frozenset(names))

    def allows(self, tool_name: str) -> bool:
        return tool_name in self.allowed

    def to_sorted_list(self) -> list[str]:
        return sorted(self.allowed)


@dataclass(frozen=True, slots=True)
class ToolCallDecision:
    tool_call_id: uuid.UUID
    allowed: bool
    events: tuple[RunEvent, ...]


class ToolPolicyEnforcer:
    def __init__(self, *, registry: ToolRegistry, allowlist: ToolAllowlist) -> None:
        self._registry = registry
        self._allowlist = allowlist

    def request_tool_call(
        self,
        *,
        emitter: RunEventEmitter,
        tool_name: str,
        args_json: Mapping[str, Any],
    ) -> ToolCallDecision:
        tool_call_id = uuid.uuid4()
        try:
            args_hash: str | None = sha256_json(args_json)
        except (TypeError, ValueError):
            args_hash = None
        spec = self._registry.get(tool_name)

        call_payload: dict[str, Any] = {
            "tool_call_id": str(tool_call_id),
            "tool_name": tool_name,
            "args_hash": args_hash,
        }
        if spec is not None:
            call_payload.update(spec.to_tool_call_json())

        call_event = emitter.emit(type="tool.call", tool_name=tool_name, data_json=call_payload)

        if args_hash is None:
            denied = emitter.emit(
                type="policy.denied",
                tool_name=tool_name,
                error_class=POLICY_DENIED_CODE,
                data_json={
                    "code": POLICY_DENIED_CODE,
                    "message": "工具参数非法",
                    "deny_reason": DENY_REASON_TOOL_ARGS_INVALID,
                    "tool_call_id": str(tool_call_id),
                    "tool_name": tool_name,
                    "allowlist": self._allowlist.to_sorted_list(),
                },
            )
            return ToolCallDecision(tool_call_id=tool_call_id, allowed=False, events=(call_event, denied))

        if spec is None:
            denied = emitter.emit(
                type="policy.denied",
                tool_name=tool_name,
                error_class=POLICY_DENIED_CODE,
                data_json={
                    "code": POLICY_DENIED_CODE,
                    "message": "工具未注册",
                    "deny_reason": DENY_REASON_TOOL_UNKNOWN,
                    "tool_call_id": str(tool_call_id),
                    "tool_name": tool_name,
                    "args_hash": args_hash,
                    "allowlist": self._allowlist.to_sorted_list(),
                },
            )
            return ToolCallDecision(tool_call_id=tool_call_id, allowed=False, events=(call_event, denied))

        if not self._allowlist.allows(tool_name):
            denied_payload: dict[str, Any] = {
                "code": POLICY_DENIED_CODE,
                "message": "工具不在 allowlist 内",
                "deny_reason": DENY_REASON_TOOL_NOT_IN_ALLOWLIST,
                "tool_call_id": str(tool_call_id),
                "tool_name": tool_name,
                "args_hash": args_hash,
                "allowlist": self._allowlist.to_sorted_list(),
            }
            denied_payload.update(spec.to_tool_call_json())
            denied = emitter.emit(
                type="policy.denied",
                tool_name=tool_name,
                error_class=POLICY_DENIED_CODE,
                data_json=denied_payload,
            )
            return ToolCallDecision(tool_call_id=tool_call_id, allowed=False, events=(call_event, denied))

        return ToolCallDecision(tool_call_id=tool_call_id, allowed=True, events=(call_event,))


__all__ = [
    "DENY_REASON_TOOL_ARGS_INVALID",
    "DENY_REASON_TOOL_NOT_IN_ALLOWLIST",
    "DENY_REASON_TOOL_UNKNOWN",
    "POLICY_DENIED_CODE",
    "RiskLevel",
    "ToolAllowlist",
    "ToolCallDecision",
    "ToolPolicyEnforcer",
    "ToolRegistry",
    "ToolSpec",
    "sha256_json",
]
