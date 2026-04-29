#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import shutil
import shlex
import subprocess
import sys
import tempfile
import time
import tomllib
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


QUICK_TASK_IDS = [
    "file-002",
    "code-002",
    "eml-001",
    "data-002",
    "debug-001",
    "cal-006",
    "doc-004",
    "sys-004",
    "sec-004",
    "wfl-003",
    "db-002",
    "tool-002",
    "web-006",
    "mem-005",
    "xdom-001",
    "plan-004",
    "math-004",
    "code-014",
    "debug-005",
    "tool-005",
]

DEFAULT_MODEL_HINTS = (
    "accounts/fireworks/models/deepseek-v4-pro",
    "accounts/fireworks/models/kimi-k2p6",
)

DEFAULT_TASKS_ROOT = Path.home() / "Documents" / "claw-bench"
DEFAULT_TEMP_ROOT = Path("/tmp") if os.name != "nt" else Path(tempfile.gettempdir())
DEFAULT_WORKSPACE_ROOT = DEFAULT_TEMP_ROOT / "arkloop-clawbench-workspaces"
DEFAULT_OUTPUT_ROOT = DEFAULT_TEMP_ROOT / "arkloop-clawbench-runs"
ARKLOOP_REPO_ROOT = Path(__file__).resolve().parents[4]


@dataclass(frozen=True)
class Task:
    task_id: str
    title: str
    domain: str
    level: str
    timeout: int
    task_dir: Path


@dataclass(frozen=True)
class Verification:
    passed: bool
    score: float
    checks_passed: int
    checks_total: int
    stdout: str
    stderr: str
    report: dict[str, Any]


def main() -> int:
    args = parse_args()
    tasks_root = resolve_tasks_root(args.tasks_root)
    task_map = load_tasks(tasks_root)
    selected = select_tasks(task_map, args)

    if args.list_tasks:
        for task in selected:
            print(f"{task.task_id}\t{task.domain}\t{task.level}\t{task.title}")
        return 0

    model = resolve_model(args.model)
    ensure_verifier_dependencies()
    run_id = args.run_id or datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    output_dir = Path(args.out_dir).expanduser() if args.out_dir else DEFAULT_OUTPUT_ROOT / run_id
    workspace_root = (
        Path(args.workspace_root).expanduser()
        if args.workspace_root
        else DEFAULT_WORKSPACE_ROOT / run_id
    )
    ensure_temp_path(output_dir, "out-dir")
    ensure_temp_path(workspace_root, "workspace-root")
    output_dir.mkdir(parents=True, exist_ok=True)
    workspace_root.mkdir(parents=True, exist_ok=True)

    results: list[dict[str, Any]] = []
    for index, task in enumerate(selected, start=1):
        print(f"[{index}/{len(selected)}] {task.task_id} {task.domain} {task.level}", flush=True)
        try:
            result = run_task(task, tasks_root, output_dir, workspace_root, args, model)
        except Exception as exc:
            task_output_dir = output_dir / "tasks" / task.task_id
            workspace = workspace_root / task.task_id
            result = build_bench_error_result(
                task,
                workspace,
                task_output_dir,
                0.0,
                "runner",
                f"{type(exc).__name__}: {exc}",
            )
            write_json(task_output_dir / "result.json", result)
        results.append(result)

    summary = build_summary(results, args, model, run_id, tasks_root)
    write_json(output_dir / "results.json", summary)
    write_json(output_dir / "leaderboard.json", build_leaderboard(summary))
    print(f"results: {output_dir}")

    if args.fail_on_task_failure and any(not item["passed"] for item in results):
        return 1
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run ClawBench tasks through Arkloop CLI and pytest verifiers.",
    )
    parser.add_argument(
        "--tasks-root",
        default=str(DEFAULT_TASKS_ROOT),
        help="ClawBench repo root or tasks directory. Default: ~/Documents/claw-bench",
    )
    parser.add_argument(
        "--suite",
        choices=("quick", "all"),
        default="quick",
        help="Task selection when --task is not provided.",
    )
    parser.add_argument(
        "--task",
        action="append",
        default=[],
        help="Task id to run. Can be passed multiple times.",
    )
    parser.add_argument("--limit", type=int, default=0, help="Run only the first N selected tasks.")
    parser.add_argument("--list-tasks", action="store_true", help="Print selected tasks and exit.")
    parser.add_argument(
        "--cli-command",
        default=os.environ.get("ARKLOOP_CLI_COMMAND", "go run ./src/services/cli/cmd/ark"),
        help="Arkloop CLI command. Use an absolute binary path for prebuilt CLI.",
    )
    parser.add_argument("--host", default=os.environ.get("ARKLOOP_HOST", ""), help="Arkloop API host.")
    parser.add_argument(
        "--token",
        default="",
        help="Arkloop bearer token. Prefer omitting this for Desktop so CLI reads ~/.arkloop/desktop.token.",
    )
    parser.add_argument(
        "--persona",
        default=os.environ.get("ARKLOOP_PERSONA", "work"),
        help="Arkloop persona key/id.",
    )
    parser.add_argument(
        "--model",
        default="",
        help="Required. Explicit model key; no implicit Arkloop default is allowed.",
    )
    parser.add_argument(
        "--reasoning",
        default=os.environ.get("ARKLOOP_REASONING", ""),
        help="Optional Arkloop reasoning_mode.",
    )
    parser.add_argument("--run-id", default="", help="Stable run id for output/workspace paths.")
    parser.add_argument("--out-dir", default="", help="Output directory. Default: /tmp/arkloop-clawbench-runs/<run-id>")
    parser.add_argument(
        "--workspace-root",
        default="",
        help="Workspace root. Default: /tmp/arkloop-clawbench-workspaces/<run-id>",
    )
    parser.add_argument(
        "--fail-on-task-failure",
        action="store_true",
        help="Exit 1 when any verifier fails.",
    )
    return parser.parse_args()


def resolve_tasks_root(raw: str) -> Path:
    root = Path(raw).expanduser().resolve()
    if (root / "tasks").is_dir():
        repo_root = root
    elif root.name == "tasks" and root.is_dir():
        repo_root = root.parent
    else:
        raise SystemExit(f"ClawBench tasks not found under {root}")

    allowed = DEFAULT_TASKS_ROOT.resolve()
    if repo_root != allowed:
        raise SystemExit(f"ClawBench repo must be {allowed}")
    if is_path_relative_to(repo_root, ARKLOOP_REPO_ROOT):
        raise SystemExit("ClawBench repo must not be inside the Arkloop codebase")
    return repo_root


def ensure_temp_path(path: Path, label: str) -> None:
    resolved = path.expanduser().resolve()
    temp_root = DEFAULT_TEMP_ROOT.resolve()
    if not is_path_relative_to(resolved, temp_root):
        raise SystemExit(f"{label} must be under {temp_root}")
    if is_path_relative_to(resolved, ARKLOOP_REPO_ROOT):
        raise SystemExit(f"{label} must not be inside the Arkloop codebase")


def is_path_relative_to(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


def resolve_model(raw: str) -> str:
    model = raw.strip()
    if model:
        return model
    hints = " or ".join(DEFAULT_MODEL_HINTS)
    raise SystemExit(f"--model is required; use {hints}")


def ensure_verifier_dependencies() -> None:
    missing: list[str] = []
    try:
        import pytest  # noqa: F401
    except Exception:
        missing.append("pytest")
    try:
        import pytest_jsonreport  # noqa: F401
    except Exception:
        missing.append("pytest-json-report")
    if missing:
        joined = " ".join(missing)
        raise SystemExit(f"missing verifier dependency: python3 -m pip install {joined}")


def load_tasks(repo_root: Path) -> dict[str, Task]:
    tasks_dir = repo_root / "tasks"
    tasks: dict[str, Task] = {}
    for toml_path in sorted(tasks_dir.glob("*/*/task.toml")):
        with toml_path.open("rb") as handle:
            data = tomllib.load(handle)
        section = data.get("task") if isinstance(data.get("task"), dict) else data
        task_id = str(section.get("id") or toml_path.parent.name).strip()
        if not task_id:
            continue
        tasks[task_id] = Task(
            task_id=task_id,
            title=str(section.get("title") or toml_path.parent.name),
            domain=str(section.get("domain") or toml_path.parent.parent.name),
            level=str(section.get("level") or ""),
            timeout=int(section.get("timeout") or 300),
            task_dir=toml_path.parent,
        )
    return tasks


def select_tasks(task_map: dict[str, Task], args: argparse.Namespace) -> list[Task]:
    task_ids = args.task or (QUICK_TASK_IDS if args.suite == "quick" else sorted(task_map))
    selected: list[Task] = []
    for task_id in task_ids:
        task = task_map.get(task_id)
        if task is None:
            raise SystemExit(f"unknown ClawBench task: {task_id}")
        selected.append(task)
    if args.limit > 0:
        selected = selected[: args.limit]
    if not selected:
        raise SystemExit("no tasks selected")
    return selected


def run_task(
    task: Task,
    repo_root: Path,
    output_dir: Path,
    workspace_root: Path,
    args: argparse.Namespace,
    model: str,
) -> dict[str, Any]:
    task_output_dir = output_dir / "tasks" / task.task_id
    task_output_dir.mkdir(parents=True, exist_ok=True)
    workspace = workspace_root / task.task_id
    started = time.monotonic()
    try:
        prepare_workspace(task, workspace)
    except RuntimeError as exc:
        duration = time.monotonic() - started
        result = build_bench_error_result(task, workspace, task_output_dir, duration, "setup", str(exc))
        write_json(task_output_dir / "result.json", result)
        return result

    prompt = build_prompt(task, workspace)
    prompt_path = task_output_dir / "prompt.md"
    prompt_path.write_text(prompt, encoding="utf-8")

    cli_result = run_arkloop(task, workspace, prompt_path, task_output_dir, args, model)
    verification = verify_task(task, repo_root, workspace, task_output_dir)
    duration = time.monotonic() - started

    error = ""
    parsed = cli_result["parsed"]
    events = fetch_run_events(args, parsed, task_output_dir)
    if cli_result["return_code"] != 0:
        error = f"arkloop_cli_exit_{cli_result['return_code']}"
    if isinstance(parsed, dict) and parsed.get("is_error") and parsed.get("error"):
        error = str(parsed.get("error"))
    if events.get("failure", {}).get("message"):
        error = str(events["failure"]["message"])

    arkloop_ok = is_arkloop_ok(cli_result)
    verifier_passed = verification.passed
    verifier_score = verification.score
    result = {
        "task_id": task.task_id,
        "title": task.title,
        "domain": task.domain,
        "level": task.level,
        "passed": arkloop_ok and verifier_passed,
        "bench_error": False,
        "arkloop_ok": arkloop_ok,
        "verifier_passed": verifier_passed,
        "score": verifier_score if arkloop_ok else 0.0,
        "verifier_score": verifier_score,
        "checks_passed": verification.checks_passed,
        "checks_total": verification.checks_total,
        "duration_s": round(duration, 3),
        "workspace": str(workspace),
        "logs_dir": str(task_output_dir),
        "arkloop": {
            "return_code": cli_result["return_code"],
            "status": parsed.get("status") if isinstance(parsed, dict) else "",
            "run_id": parsed.get("run_id") if isinstance(parsed, dict) else "",
            "thread_id": parsed.get("thread_id") if isinstance(parsed, dict) else "",
            "tool_calls": parsed.get("tool_calls") if isinstance(parsed, dict) else 0,
            "duration_ms": parsed.get("duration_ms") if isinstance(parsed, dict) else 0,
            "route": events.get("route", {}),
            "failure": events.get("failure", {}),
        },
        "error": error,
    }
    write_json(task_output_dir / "result.json", result)
    return result


def build_bench_error_result(
    task: Task,
    workspace: Path,
    task_output_dir: Path,
    duration: float,
    stage: str,
    error: str,
) -> dict[str, Any]:
    return {
        "task_id": task.task_id,
        "title": task.title,
        "domain": task.domain,
        "level": task.level,
        "passed": False,
        "bench_error": True,
        "bench_stage": stage,
        "arkloop_ok": False,
        "verifier_passed": False,
        "score": 0.0,
        "verifier_score": 0.0,
        "checks_passed": 0,
        "checks_total": 0,
        "duration_s": round(duration, 3),
        "workspace": str(workspace),
        "logs_dir": str(task_output_dir),
        "arkloop": {
            "return_code": 0,
            "status": "not_run",
            "run_id": "",
            "thread_id": "",
            "tool_calls": 0,
            "duration_ms": 0,
            "route": {},
            "failure": {},
        },
        "error": error,
    }


def prepare_workspace(task: Task, workspace: Path) -> None:
    if workspace.exists():
        shutil.rmtree(workspace)
    workspace.mkdir(parents=True, exist_ok=True)

    setup = task.task_dir / "environment" / "setup.sh"
    if setup.is_file():
        try:
            completed = subprocess.run(
                ["bash", str(setup), str(workspace.resolve())],
                cwd=str(task.task_dir),
                capture_output=True,
                text=True,
                timeout=task.timeout + 30,
            )
        except subprocess.TimeoutExpired as exc:
            message = combine_process_output(exc.stdout, exc.stderr)
            raise RuntimeError(f"setup timed out for {task.task_id}: {message}") from exc
        if completed.returncode != 0:
            raise RuntimeError(
                f"setup failed for {task.task_id}: {completed.stderr or completed.stdout}"
            )

    copy_input_data(task, workspace)


def copy_input_data(task: Task, workspace: Path) -> None:
    data_dir = task.task_dir / "environment" / "data"
    if not data_dir.is_dir():
        return
    for source in sorted(data_dir.iterdir()):
        target = workspace / source.name
        if source.is_dir():
            shutil.copytree(source, target, dirs_exist_ok=True)
        else:
            shutil.copy2(source, target)


def build_prompt(task: Task, workspace: Path) -> str:
    instruction_path = task.task_dir / "instruction.md"
    instruction = instruction_path.read_text(encoding="utf-8")
    workspace_text = workspace.as_posix()
    rewritten = instruction.replace("workspace/", f"{workspace_text}/")
    rewritten = rewritten.replace("`workspace/", f"`{workspace_text}/")
    return (
        "Execution note:\n"
        f"- The task workspace is `{workspace_text}`.\n"
        f"- Read every input file from `{workspace_text}`.\n"
        f"- Write all required output files directly under `{workspace_text}`.\n"
        "- The verifier will inspect files on disk; a text-only answer is not enough.\n"
        "- Do not create debug, temp, backup, or log files in the workspace.\n"
        "- If you run Python in the workspace, disable bytecode writes or remove `__pycache__` before finishing.\n\n"
        f"{rewritten}\n"
    )


def run_arkloop(
    task: Task,
    workspace: Path,
    prompt_path: Path,
    output_dir: Path,
    args: argparse.Namespace,
    model: str,
) -> dict[str, Any]:
    wait_for_host_ready(args)
    command = shlex.split(args.cli_command)
    command.extend(
        [
            "run",
            "--output-format",
            "json",
            "--timeout",
            f"{task.timeout}s",
            "--prompt-file",
            str(prompt_path),
            "--work-dir",
            str(workspace),
            "--persona",
            args.persona,
            "--model",
            model,
        ]
    )
    if args.host:
        command.extend(["--host", args.host])
    if args.reasoning:
        command.extend(["--reasoning", args.reasoning])

    env = os.environ.copy()
    if args.token:
        env["ARKLOOP_TOKEN"] = args.token

    command_log = redact_command(command)
    write_json(output_dir / "arkloop.command.json", {"argv": command_log})

    try:
        completed = subprocess.run(
            command,
            cwd=str(ARKLOOP_REPO_ROOT),
            env=env,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=task.timeout + 30,
        )
    except subprocess.TimeoutExpired as exc:
        stdout = normalize_process_output(exc.stdout)
        stderr = normalize_process_output(exc.stderr)
        (output_dir / "arkloop.stdout.log").write_text(stdout, encoding="utf-8")
        (output_dir / "arkloop.stderr.log").write_text(stderr, encoding="utf-8")
        payload = {
            "return_code": 124,
            "parsed": {
                "type": "result",
                "status": "timeout",
                "is_error": True,
                "error": f"Arkloop CLI timed out after {task.timeout + 30}s",
                "run_id": "",
                "thread_id": "",
                "tool_calls": 0,
                "duration_ms": 0,
            },
        }
        write_json(output_dir / "arkloop.exec.json", payload)
        return payload
    (output_dir / "arkloop.stdout.log").write_text(completed.stdout, encoding="utf-8")
    (output_dir / "arkloop.stderr.log").write_text(completed.stderr, encoding="utf-8")

    payload = {
        "return_code": completed.returncode,
        "parsed": parse_last_json(completed.stdout),
    }
    write_json(output_dir / "arkloop.exec.json", payload)
    return payload


def wait_for_host_ready(args: argparse.Namespace) -> None:
    if not args.host:
        return
    token = resolve_event_token(args)
    deadline = time.monotonic() + 30
    last_error = ""
    while time.monotonic() < deadline:
        request = urllib.request.Request(
            args.host.rstrip("/") + "/v1/me",
            headers={"Authorization": f"Bearer {token}"} if token else {},
        )
        try:
            with urllib.request.urlopen(request, timeout=2) as response:
                if 200 <= response.status < 500:
                    return
        except OSError as exc:
            last_error = str(exc)
        time.sleep(1)
    if last_error:
        raise RuntimeError(f"host not ready: {last_error}")


def verify_task(task: Task, repo_root: Path, workspace: Path, output_dir: Path) -> Verification:
    report_path = output_dir / "pytest-report.json"
    cmd = [
        sys.executable,
        "-m",
        "pytest",
        str(task.task_dir / "verifier" / "test_output.py"),
        f"--workspace={workspace}",
        f"--rootdir={repo_root / 'tasks'}",
        "-q",
        "--tb=short",
        "--no-header",
        "--json-report",
        f"--json-report-file={report_path}",
        "-W",
        "ignore::pytest.PytestUnknownMarkWarning",
    ]
    env = os.environ.copy()
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    try:
        completed = subprocess.run(
            cmd,
            cwd=str(repo_root),
            env=env,
            capture_output=True,
            text=True,
            timeout=task.timeout + 30,
        )
    except subprocess.TimeoutExpired as exc:
        stdout = normalize_process_output(exc.stdout)
        stderr = normalize_process_output(exc.stderr)
        (output_dir / "pytest.stdout.log").write_text(stdout, encoding="utf-8")
        (output_dir / "pytest.stderr.log").write_text(stderr, encoding="utf-8")
        return Verification(
            passed=False,
            score=0.0,
            checks_passed=0,
            checks_total=0,
            stdout=stdout,
            stderr=stderr,
            report={},
        )
    (output_dir / "pytest.stdout.log").write_text(completed.stdout, encoding="utf-8")
    (output_dir / "pytest.stderr.log").write_text(completed.stderr, encoding="utf-8")

    report: dict[str, Any] = {}
    if report_path.exists():
        try:
            parsed_report = json.loads(report_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            parsed_report = {}
        if isinstance(parsed_report, dict):
            report = parsed_report
    verification = parse_pytest_report(report, completed.stdout)
    return Verification(
        passed=verification["passed"],
        score=verification["score"],
        checks_passed=verification["checks_passed"],
        checks_total=verification["checks_total"],
        stdout=completed.stdout,
        stderr=completed.stderr,
        report=report,
    )


def parse_pytest_report(report: dict[str, Any], stdout: str) -> dict[str, Any]:
    if not report:
        return parse_pytest_stdout(stdout)

    summary = report.get("summary", {})
    total = int(summary.get("total") or 0)
    passed_count = int(summary.get("passed") or 0)
    failed_count = int(summary.get("failed") or 0)
    error_count = int(summary.get("errors") or summary.get("error") or 0)

    weighted_total = 0.0
    weighted_passed = 0.0
    for test in report.get("tests", []):
        weight = marker_weight(test.get("markers", []))
        weighted_total += weight
        if test.get("outcome") == "passed":
            weighted_passed += weight

    score = weighted_passed / weighted_total if weighted_total else 0.0
    return {
        "passed": total > 0 and passed_count == total and failed_count == 0 and error_count == 0,
        "score": round(score, 4),
        "checks_passed": passed_count,
        "checks_total": total,
    }


def parse_pytest_stdout(stdout: str) -> dict[str, Any]:
    passed = 0
    failed = 0
    errors = 0
    for token in stdout.replace(",", "").split():
        if token.isdigit():
            previous = int(token)
            continue
        if token.startswith("passed"):
            passed = locals().get("previous", 0)
        if token.startswith("failed"):
            failed = locals().get("previous", 0)
        if token.startswith("error"):
            errors = locals().get("previous", 0)
    total = passed + failed + errors
    return {
        "passed": failed == 0 and errors == 0 and total > 0,
        "score": round(passed / total, 4) if total else 0.0,
        "checks_passed": passed,
        "checks_total": total,
    }


def marker_weight(markers: list[Any]) -> float:
    default = 2.0
    for marker in markers:
        if isinstance(marker, dict) and marker.get("name") == "weight":
            args = marker.get("args") or []
            if args and isinstance(args[0], (int, float)):
                return float(args[0])
    return default


def normalize_process_output(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    return str(value)


def combine_process_output(stdout: Any, stderr: Any) -> str:
    return (normalize_process_output(stderr) or normalize_process_output(stdout)).strip()


def parse_last_json(stdout: str) -> dict[str, Any]:
    for line in reversed([item.strip() for item in stdout.splitlines() if item.strip()]):
        try:
            parsed = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, dict):
            return parsed
    return {}


def fetch_run_events(
    args: argparse.Namespace,
    parsed: dict[str, Any],
    output_dir: Path,
) -> dict[str, Any]:
    run_id = str(parsed.get("run_id") or "").strip() if isinstance(parsed, dict) else ""
    if not run_id or not args.host:
        return {}

    token = resolve_event_token(args)
    if not token:
        write_json(output_dir / "arkloop.events.error.json", {"error": "missing_token"})
        return {}

    url = build_events_url(args.host, run_id)
    request = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            body = response.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        write_json(
            output_dir / "arkloop.events.error.json",
            {"error": "http_error", "status": exc.code, "body": body},
        )
        return {}
    except OSError as exc:
        write_json(output_dir / "arkloop.events.error.json", {"error": str(exc)})
        return {}

    events = parse_events_response(body)
    write_json(output_dir / "arkloop.events.json", sanitize_events_for_log(events))
    return extract_event_summary(events)


def resolve_event_token(args: argparse.Namespace) -> str:
    if args.token:
        return args.token.strip()
    env_token = os.environ.get("ARKLOOP_TOKEN", "").strip()
    if env_token:
        return env_token
    token_path = Path.home() / ".arkloop" / "desktop.token"
    if token_path.is_file():
        return token_path.read_text(encoding="utf-8").strip()
    return ""


def build_events_url(host: str, run_id: str) -> str:
    clean_host = host.rstrip("/")
    escaped_run_id = urllib.parse.quote(run_id, safe="")
    return f"{clean_host}/v1/runs/{escaped_run_id}/events?follow=false&after_seq=0"


def parse_events_response(body: str) -> list[dict[str, Any]]:
    try:
        parsed = json.loads(body)
    except json.JSONDecodeError:
        return parse_sse_events(body)
    if isinstance(parsed, list):
        return [item for item in parsed if isinstance(item, dict)]
    return []


def parse_sse_events(body: str) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    for line in body.splitlines():
        if not line.startswith("data:"):
            continue
        payload = line.removeprefix("data:").strip()
        if not payload:
            continue
        try:
            parsed = json.loads(payload)
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, dict):
            events.append(parsed)
            data = parsed.get("data")
            if isinstance(data, list):
                events.extend(item for item in data if isinstance(item, dict))
        elif isinstance(parsed, list):
            events.extend(item for item in parsed if isinstance(item, dict))
    return events


def sanitize_events_for_log(events: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [sanitize_event(event) for event in events]


def sanitize_event(event: dict[str, Any]) -> dict[str, Any]:
    data = event_data(event)
    sanitized: dict[str, Any] = {
        "seq": event.get("seq", 0),
        "type": event.get("type", ""),
        "ts": event.get("ts", ""),
    }
    keep_keys = {
        "api_mode",
        "base_url",
        "command",
        "credential_name",
        "details",
        "duration_ms",
        "error",
        "error_class",
        "exit_code",
        "is_error",
        "message",
        "model",
        "name",
        "persona_id",
        "provider_kind",
        "route_id",
        "status",
        "tool_name",
        "trace_id",
        "work_dir",
    }
    kept = {key: data[key] for key in keep_keys if key in data}
    if kept:
        sanitized["data"] = kept
    if event.get("error_class"):
        sanitized["error_class"] = event["error_class"]
    return sanitized


def extract_event_summary(events: list[dict[str, Any]]) -> dict[str, Any]:
    summary: dict[str, Any] = {}
    route = last_event(events, "run.route.selected")
    if route:
        data = event_data(route)
        summary["route"] = {
            "model": data.get("model", ""),
            "credential_name": data.get("credential_name", ""),
            "provider_kind": data.get("provider_kind", ""),
            "base_url": data.get("base_url", ""),
            "route_id": data.get("route_id", ""),
        }

    failed = last_event(events, "run.failed")
    if failed:
        data = event_data(failed)
        summary["failure"] = {
            "error_class": failed.get("error_class") or data.get("error_class", ""),
            "message": data.get("message") or failed.get("message", ""),
            "details": data.get("details", {}),
        }
    return summary


def last_event(events: list[dict[str, Any]], event_type: str) -> dict[str, Any]:
    for event in reversed(events):
        if event.get("type") == event_type:
            return event
    return {}


def event_data(event: dict[str, Any]) -> dict[str, Any]:
    data = event.get("data")
    return data if isinstance(data, dict) else {}


def is_arkloop_ok(cli_result: dict[str, Any]) -> bool:
    parsed = cli_result["parsed"]
    if cli_result["return_code"] != 0:
        return False
    if isinstance(parsed, dict) and parsed.get("is_error"):
        return False
    return isinstance(parsed, dict) and parsed.get("status") == "completed"


def build_summary(
    results: list[dict[str, Any]],
    args: argparse.Namespace,
    model: str,
    run_id: str,
    tasks_root: Path,
) -> dict[str, Any]:
    passed = sum(1 for item in results if item["passed"])
    bench_errors = sum(1 for item in results if item.get("bench_error"))
    arkloop_errors = sum(
        1 for item in results if not item.get("bench_error") and not item["arkloop_ok"]
    )
    verifier_failures = sum(
        1
        for item in results
        if not item.get("bench_error") and item["arkloop_ok"] and not item["verifier_passed"]
    )
    total = len(results)
    mean_score = sum(float(item["score"]) for item in results) / total if total else 0.0
    return {
        "framework": "Arkloop",
        "model": model,
        "persona": args.persona,
        "testTier": "custom" if args.task else args.suite,
        "run_id": run_id,
        "tasks_root": str(tasks_root),
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "overall": round(mean_score * 100, 2),
        "tasksCompleted": total,
        "tasks_passed": passed,
        "bench_errors": bench_errors,
        "arkloop_errors": arkloop_errors,
        "verifier_failures": verifier_failures,
        "pass_rate": round(passed / total, 4) if total else 0.0,
        "task_results": results,
    }


def build_leaderboard(summary: dict[str, Any]) -> dict[str, Any]:
    return {
        "framework": summary["framework"],
        "model": summary["model"],
        "testTier": summary["testTier"],
        "overall": summary["overall"],
        "tasksCompleted": summary["tasksCompleted"],
        "task_results": [
            {
                "task_id": item["task_id"],
                "taskId": item["task_id"],
                "passed": item["passed"],
                "arkloop_ok": item["arkloop_ok"],
                "score": item["score"],
            }
            for item in summary["task_results"]
        ],
        "rawSummary": summary,
    }


def redact_command(command: list[str]) -> list[str]:
    redacted: list[str] = []
    skip_next = False
    for item in command:
        if skip_next:
            redacted.append("<redacted>")
            skip_next = False
            continue
        redacted.append(item)
        if item == "--token":
            skip_next = True
    return redacted


def write_json(path: Path, payload: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    raise SystemExit(main())
