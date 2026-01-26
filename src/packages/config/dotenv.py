from __future__ import annotations

import os
from pathlib import Path
import re
from threading import Lock
from typing import Optional, Tuple

_DOTENV_ENABLE_ENV = "ARKLOOP_LOAD_DOTENV"
_DOTENV_FILE_ENV = "ARKLOOP_DOTENV_FILE"

_TRUTHY = {"1", "true", "yes", "y", "on"}
_FALSY = {"0", "false", "no", "n", "off"}

_LOAD_LOCK = Lock()
_LOADED_DOTENV_PATHS: set[str] = set()


def _parse_bool(value: str) -> bool:
    normalized = value.strip().casefold()
    if normalized in _TRUTHY:
        return True
    if normalized in _FALSY:
        return False
    raise ValueError(f"无法解析布尔值: {value!r}")


def _find_repo_root(start: Path) -> Path:
    current = start.resolve()
    candidates = (current, *current.parents)
    for directory in candidates:
        if (directory / "pyproject.toml").is_file() or (directory / ".git").exists():
            return directory
    return current


def _resolve_dotenv_path() -> Optional[Path]:
    configured = os.getenv(_DOTENV_FILE_ENV)
    if configured:
        path = Path(configured).expanduser()
        return path if path.is_file() else None

    repo_root = _find_repo_root(Path.cwd())
    path = repo_root / ".env"
    return path if path.is_file() else None


def _parse_dotenv_line(raw_line: str) -> Optional[Tuple[str, str]]:
    line = raw_line.strip()
    if not line or line.startswith("#"):
        return None

    if line.startswith("export "):
        line = line[len("export ") :].lstrip()

    if "=" not in line:
        return None

    key, _, value = line.partition("=")
    key = key.strip()
    if not key:
        return None

    if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
        return None

    value = value.strip()
    if value and value[0] in {"'", '"'}:
        quote = value[0]
        if len(value) >= 2 and value[-1] == quote:
            value = value[1:-1]
        return key, value

    comment_match = re.search(r"\s+#", value)
    if comment_match:
        value = value[: comment_match.start()].rstrip()
    return key, value


def _load_dotenv(path: Path, *, override: bool) -> None:
    content = path.read_text(encoding="utf-8-sig")
    for raw_line in content.splitlines():
        parsed = _parse_dotenv_line(raw_line)
        if parsed is None:
            continue
        key, value = parsed
        if not override and key in os.environ:
            continue
        os.environ[key] = value


def load_dotenv_if_enabled(*, override: bool = False) -> bool:
    enabled = os.getenv(_DOTENV_ENABLE_ENV)
    if enabled is None:
        return False
    if not _parse_bool(enabled):
        return False

    dotenv_path = _resolve_dotenv_path()
    if dotenv_path is None:
        return False

    resolved = str(dotenv_path.resolve())
    with _LOAD_LOCK:
        if resolved in _LOADED_DOTENV_PATHS:
            return False
        _load_dotenv(dotenv_path, override=override)
        _LOADED_DOTENV_PATHS.add(resolved)
        return True

