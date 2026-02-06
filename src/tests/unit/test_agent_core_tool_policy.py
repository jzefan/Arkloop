from __future__ import annotations

from datetime import datetime, timezone
import hashlib
import json
import uuid

from packages.agent_core import (
    DENY_REASON_TOOL_NOT_IN_ALLOWLIST,
    POLICY_DENIED_CODE,
    RunEventEmitter,
    ToolAllowlist,
    ToolPolicyEnforcer,
    ToolRegistry,
    ToolSpec,
)


class _FakeEventIdFactory:
    def __init__(self) -> None:
        self._next_int = 1

    def __call__(self) -> uuid.UUID:
        value = uuid.UUID(int=self._next_int)
        self._next_int += 1
        return value


def _fixed_clock() -> datetime:
    return datetime(2025, 1, 1, 0, 0, 0, tzinfo=timezone.utc)


def _sha256_hex(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def test_tool_call_is_denied_when_not_in_allowlist_and_emits_replayable_events() -> None:
    run_id = uuid.UUID(int=42)
    emitter = RunEventEmitter(
        run_id=run_id,
        trace_id="b" * 32,
        event_id_factory=_FakeEventIdFactory(),
        clock=_fixed_clock,
    )

    registry = ToolRegistry(
        specs=[
            ToolSpec(name="echo", version="1", description="回显", risk_level="low"),
            ToolSpec(
                name="shell",
                version="1",
                description="执行命令",
                risk_level="high",
                required_scopes=("os.exec",),
                side_effects=True,
            ),
        ]
    )
    allowlist = ToolAllowlist.from_names(["echo"])
    enforcer = ToolPolicyEnforcer(registry=registry, allowlist=allowlist)

    args_json = {"cmd": "echo hello", "cwd": "."}
    decision = enforcer.request_tool_call(emitter=emitter, tool_name="shell", args_json=args_json)

    assert decision.allowed is False
    assert [event.seq for event in decision.events] == [1, 2]
    assert [event.type for event in decision.events] == ["tool.call", "policy.denied"]

    tool_call, denied = decision.events

    expected_args_hash = _sha256_hex(
        json.dumps(args_json, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    )
    assert tool_call.data_json["args_hash"] == expected_args_hash
    assert denied.data_json["args_hash"] == expected_args_hash

    assert tool_call.data_json["tool_call_id"] == denied.data_json["tool_call_id"]
    uuid.UUID(tool_call.data_json["tool_call_id"])

    assert denied.error_class == POLICY_DENIED_CODE
    assert denied.data_json["code"] == POLICY_DENIED_CODE
    assert denied.data_json["deny_reason"] == DENY_REASON_TOOL_NOT_IN_ALLOWLIST
    assert denied.data_json["allowlist"] == ["echo"]

    assert tool_call.data_json["risk_level"] == "high"
    assert tool_call.data_json["required_scopes"] == ["os.exec"]
    assert tool_call.data_json["side_effects"] is True
    assert tool_call.data_json["tool_version"] == "1"

    assert denied.data_json["risk_level"] == "high"
    assert denied.data_json["required_scopes"] == ["os.exec"]
    assert denied.data_json["side_effects"] is True
    assert denied.data_json["tool_version"] == "1"

    assert all(event.data_json["trace_id"] == "b" * 32 for event in decision.events)

    resumed = [event for event in decision.events if event.seq > 1]
    assert [(event.seq, event.type) for event in resumed] == [(2, "policy.denied")]


def test_tool_policy_enforcer_reuses_given_tool_call_id() -> None:
    run_id = uuid.UUID(int=7)
    emitter = RunEventEmitter(
        run_id=run_id,
        trace_id="c" * 32,
        event_id_factory=_FakeEventIdFactory(),
        clock=_fixed_clock,
    )
    registry = ToolRegistry(
        specs=[ToolSpec(name="echo", version="1", description="回显", risk_level="low")]
    )
    allowlist = ToolAllowlist.from_names(["echo"])
    enforcer = ToolPolicyEnforcer(registry=registry, allowlist=allowlist)

    decision = enforcer.request_tool_call(
        emitter=emitter,
        tool_name="echo",
        args_json={"text": "hi"},
        tool_call_id="external_call_1",
    )

    assert decision.allowed is True
    assert decision.tool_call_id == "external_call_1"
    assert [event.type for event in decision.events] == ["tool.call"]
    assert decision.events[0].data_json["tool_call_id"] == "external_call_1"


def test_tool_policy_enforcer_generates_tool_call_id_for_blank_input() -> None:
    emitter = RunEventEmitter(
        run_id=uuid.UUID(int=9),
        trace_id="d" * 32,
        event_id_factory=_FakeEventIdFactory(),
        clock=_fixed_clock,
    )
    registry = ToolRegistry(
        specs=[ToolSpec(name="echo", version="1", description="回显", risk_level="low")]
    )
    enforcer = ToolPolicyEnforcer(
        registry=registry,
        allowlist=ToolAllowlist.from_names(["echo"]),
    )

    decision = enforcer.request_tool_call(
        emitter=emitter,
        tool_name="echo",
        args_json={"text": "hi"},
        tool_call_id="   ",
    )

    assert decision.allowed is True
    assert decision.tool_call_id
    uuid.UUID(decision.tool_call_id)
    assert decision.events[0].data_json["tool_call_id"] == decision.tool_call_id
