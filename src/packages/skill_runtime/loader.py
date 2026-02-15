from __future__ import annotations

import ast
import json
from pathlib import Path
import re
from typing import Any, Mapping

from .schema import SkillBudgets, SkillDefinition, SkillRegistry

_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$")
_BUDGET_KEYS = {"max_iterations", "max_output_tokens", "tool_timeout_ms", "tool_budget"}


def builtin_skills_root() -> Path:
    current = Path(__file__).resolve()
    for parent in current.parents:
        if parent.name == "src":
            return parent / "skills"
    raise RuntimeError("未找到 src 目录，无法定位 skills 根目录")


def load_skill_registry(root: Path) -> SkillRegistry:
    registry = SkillRegistry()
    if not root.exists():
        return registry
    if not root.is_dir():
        raise ValueError("skills 根目录必须为目录")

    for entry in sorted(root.iterdir(), key=lambda item: item.name):
        if not entry.is_dir():
            continue
        yaml_path = entry / "skill.yaml"
        prompt_path = entry / "prompt.md"
        if not yaml_path.is_file() or not prompt_path.is_file():
            continue

        definition = _load_single_skill(yaml_path=yaml_path, prompt_path=prompt_path)
        registry.register(definition)

    return registry


def _load_single_skill(*, yaml_path: Path, prompt_path: Path) -> SkillDefinition:
    try:
        raw_yaml = _parse_skill_yaml(yaml_path.read_text(encoding="utf-8"), path=yaml_path)
    except Exception as exc:
        raise ValueError(f"解析 skill.yaml 失败: {yaml_path}") from exc

    obj = _as_mapping(raw_yaml, label="skill.yaml")

    skill_id = _as_id(obj.get("id"), label="id")
    version = _as_id(obj.get("version"), label="version")
    title = _as_non_empty_str(obj.get("title"), label="title")
    description = _optional_str(obj.get("description"), label="description")
    tool_allowlist = _as_tool_allowlist(obj.get("tool_allowlist"))
    budgets = _as_budgets(obj.get("budgets"))

    prompt_md = prompt_path.read_text(encoding="utf-8").strip()
    if not prompt_md:
        raise ValueError(f"prompt.md 不能为空: {prompt_path}")

    return SkillDefinition(
        id=skill_id,
        version=version,
        title=title,
        description=description,
        tool_allowlist=tool_allowlist,
        budgets=budgets,
        prompt_md=prompt_md,
    )


def _parse_skill_yaml(text: str, *, path: Path) -> object:
    stripped = text.strip()
    if not stripped:
        raise ValueError(f"skill.yaml 不能为空: {path}")
    if stripped.startswith("{") or stripped.startswith("["):
        return json.loads(stripped)
    try:
        import yaml  # type: ignore[import-not-found]
    except ModuleNotFoundError:
        return _parse_simple_yaml(stripped, path=path)
    return yaml.safe_load(stripped)


def _parse_simple_yaml(text: str, *, path: Path) -> object:
    lines = _tokenize_yaml_lines(text, path=path)
    if not lines:
        raise ValueError(f"skill.yaml 为空: {path}")
    value, index = _parse_yaml_block(lines, 0, indent=lines[0][0], path=path)
    if index != len(lines):
        raise ValueError(f"skill.yaml 解析不完整: {path}")
    return value


def _tokenize_yaml_lines(text: str, *, path: Path) -> list[tuple[int, str, int]]:
    tokens: list[tuple[int, str, int]] = []
    for lineno, raw in enumerate(text.splitlines(), start=1):
        if "\t" in raw:
            raise ValueError(f"不支持 tab 缩进: {path}:{lineno}")
        stripped = raw.strip()
        if not stripped or stripped.startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip(" "))
        tokens.append((indent, raw.lstrip(" "), lineno))
    return tokens


def _parse_yaml_block(
    lines: list[tuple[int, str, int]],
    index: int,
    *,
    indent: int,
    path: Path,
) -> tuple[object, int]:
    if index >= len(lines):
        return {}, index

    current_indent, content, _lineno = lines[index]
    if current_indent < indent:
        return {}, index

    if content.startswith("-"):
        return _parse_yaml_list(lines, index, indent=indent, path=path)
    return _parse_yaml_mapping(lines, index, indent=indent, path=path)


def _parse_yaml_list(
    lines: list[tuple[int, str, int]],
    index: int,
    *,
    indent: int,
    path: Path,
) -> tuple[list[object], int]:
    values: list[object] = []
    while index < len(lines):
        current_indent, content, lineno = lines[index]
        if current_indent < indent:
            break
        if current_indent != indent:
            raise ValueError(f"缩进不一致: {path}:{lineno}")
        if not content.startswith("-"):
            raise ValueError(f"预期 list item: {path}:{lineno}")
        item = content[1:].strip()
        if not item:
            raise ValueError(f"不支持空 list item: {path}:{lineno}")
        values.append(_parse_scalar(item))
        index += 1
    return values, index


def _parse_yaml_mapping(
    lines: list[tuple[int, str, int]],
    index: int,
    *,
    indent: int,
    path: Path,
) -> tuple[dict[str, object], int]:
    values: dict[str, object] = {}
    while index < len(lines):
        current_indent, content, lineno = lines[index]
        if current_indent < indent:
            break
        if current_indent != indent:
            raise ValueError(f"缩进不一致: {path}:{lineno}")

        key, sep, rest = content.partition(":")
        if not sep:
            raise ValueError(f"预期 key: value: {path}:{lineno}")
        key = key.strip()
        if not key:
            raise ValueError(f"空 key: {path}:{lineno}")

        tail = rest.strip()
        if tail:
            values[key] = _parse_inline_value(tail)
            index += 1
            continue

        index += 1
        if index >= len(lines):
            raise ValueError(f"缺少嵌套内容: {path}:{lineno}")
        child_indent, _child_content, child_lineno = lines[index]
        if child_indent <= indent:
            raise ValueError(f"缺少嵌套内容: {path}:{child_lineno}")
        child, index = _parse_yaml_block(lines, index, indent=child_indent, path=path)
        values[key] = child
    return values, index


def _parse_inline_value(value: str) -> object:
    cleaned = value.strip()
    if cleaned.startswith("[") and cleaned.endswith("]"):
        inner = cleaned[1:-1].strip()
        if not inner:
            return []
        parts = [part.strip() for part in inner.split(",")]
        return [_parse_scalar(part) for part in parts if part]
    return _parse_scalar(cleaned)


def _parse_scalar(value: str) -> object:
    cleaned = value.strip()
    if not cleaned:
        return ""
    lowered = cleaned.casefold()
    if lowered == "true":
        return True
    if lowered == "false":
        return False
    if lowered in {"null", "~"}:
        return None
    if re.fullmatch(r"-?[0-9]+", cleaned):
        return int(cleaned)
    if (cleaned.startswith('"') and cleaned.endswith('"')) or (cleaned.startswith("'") and cleaned.endswith("'")):
        try:
            parsed = ast.literal_eval(cleaned)
        except Exception:
            return cleaned.strip('"').strip("'")
        return parsed if isinstance(parsed, str) else str(parsed)
    return cleaned


def _as_mapping(value: object, *, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ValueError(f"{label} 必须为对象")
    return value


def _as_non_empty_str(value: object, *, label: str) -> str:
    if not isinstance(value, str):
        raise ValueError(f"{label} 必须为字符串")
    cleaned = value.strip()
    if not cleaned:
        raise ValueError(f"{label} 不能为空")
    return cleaned


def _optional_str(value: object, *, label: str) -> str | None:
    if value is None:
        return None
    return _as_non_empty_str(value, label=label)


def _as_id(value: object, *, label: str) -> str:
    cleaned = _as_non_empty_str(value, label=label)
    if not _ID_RE.fullmatch(cleaned):
        raise ValueError(f"{label} 不合法: {cleaned}")
    return cleaned


def _as_tool_allowlist(value: object) -> tuple[str, ...]:
    if value is None:
        return ()
    if not isinstance(value, list):
        raise ValueError("tool_allowlist 必须为数组")
    items: list[str] = []
    seen: set[str] = set()
    for index, raw in enumerate(value):
        if not isinstance(raw, str):
            raise ValueError(f"tool_allowlist[{index}] 必须为字符串")
        cleaned = raw.strip()
        if not cleaned:
            raise ValueError(f"tool_allowlist[{index}] 不能为空")
        if not _ID_RE.fullmatch(cleaned):
            raise ValueError(f"tool_allowlist[{index}] 不合法: {cleaned}")
        if cleaned in seen:
            continue
        seen.add(cleaned)
        items.append(cleaned)
    return tuple(items)


def _as_positive_int(value: object, *, label: str) -> int:
    if not isinstance(value, int):
        raise ValueError(f"{label} 必须为整数")
    if value <= 0:
        raise ValueError(f"{label} 必须为正整数")
    return int(value)


def _as_budgets(value: object) -> SkillBudgets:
    if value is None:
        return SkillBudgets()
    if not isinstance(value, Mapping):
        raise ValueError("budgets 必须为对象")

    unknown = sorted(set(value.keys()).difference(_BUDGET_KEYS))
    if unknown:
        raise ValueError(f"budgets 包含不支持字段: {unknown}")

    max_iterations = value.get("max_iterations")
    if max_iterations is not None:
        max_iterations = _as_positive_int(max_iterations, label="budgets.max_iterations")

    max_output_tokens = value.get("max_output_tokens")
    if max_output_tokens is not None:
        max_output_tokens = _as_positive_int(max_output_tokens, label="budgets.max_output_tokens")

    tool_timeout_ms = value.get("tool_timeout_ms")
    if tool_timeout_ms is not None:
        tool_timeout_ms = _as_positive_int(tool_timeout_ms, label="budgets.tool_timeout_ms")

    tool_budget = value.get("tool_budget") or {}
    if not isinstance(tool_budget, Mapping):
        raise ValueError("budgets.tool_budget 必须为对象")

    return SkillBudgets(
        max_iterations=max_iterations,  # type: ignore[arg-type]
        max_output_tokens=max_output_tokens,  # type: ignore[arg-type]
        tool_timeout_ms=tool_timeout_ms,  # type: ignore[arg-type]
        tool_budget=dict(tool_budget),
    )


__all__ = [
    "builtin_skills_root",
    "load_skill_registry",
]
