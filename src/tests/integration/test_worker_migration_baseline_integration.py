from __future__ import annotations

from pathlib import Path
import json

import pytest

pytestmark = pytest.mark.integration

_TERMINAL_EVENT_TYPES = {"run.completed", "run.failed", "run.cancelled"}


def _repo_root() -> Path:
    current = Path(__file__).resolve()
    for parent in current.parents:
        if (parent / "pyproject.toml").exists():
            return parent
    raise AssertionError("未找到仓库根目录（pyproject.toml）")


def _golden_path() -> Path:
    return _repo_root() / "src/tests/contracts/golden/run-events/run_execute_success.v1.json"


def _load_golden_events() -> list[dict[str, object]]:
    payload = json.loads(_golden_path().read_text(encoding="utf-8"))
    events = payload.get("events")
    if not isinstance(events, list) or not events:
        raise AssertionError("golden.events 必须为非空数组")
    normalized: list[dict[str, object]] = []
    for index, item in enumerate(events):
        if not isinstance(item, dict):
            raise AssertionError(f"golden.events[{index}] 必须为对象")
        normalized.append(item)
    return normalized


def test_run_execute_golden_seq_is_strictly_increasing() -> None:
    events = _load_golden_events()
    seqs = [event.get("seq") for event in events]
    if not all(isinstance(seq, int) for seq in seqs):
        raise AssertionError("golden.seq 必须全部为整数")
    typed = [int(seq) for seq in seqs]
    assert typed == sorted(typed)
    assert len(typed) == len(set(typed))


def test_run_execute_golden_has_single_terminal_event_at_tail() -> None:
    events = _load_golden_events()
    types = [event.get("type") for event in events]
    if not all(isinstance(item, str) and item for item in types):
        raise AssertionError("golden.type 必须全部为非空字符串")
    typed = [str(item) for item in types]
    terminal = [item for item in typed if item in _TERMINAL_EVENT_TYPES]
    assert len(terminal) == 1
    assert typed[-1] in _TERMINAL_EVENT_TYPES


def test_run_execute_golden_preserves_critical_event_order() -> None:
    events = _load_golden_events()
    types = [str(event["type"]) for event in events]

    started_index = types.index("run.started")
    received_index = types.index("worker.job.received")

    terminal_index = -1
    for event_type in _TERMINAL_EVENT_TYPES:
        if event_type in types:
            terminal_index = types.index(event_type)
            break

    assert terminal_index >= 0
    assert started_index < received_index < terminal_index
