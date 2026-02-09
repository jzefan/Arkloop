from __future__ import annotations

import os
from pathlib import Path
import re

import pytest


def _selects_marker(markexpr: str, marker: str) -> bool:
    return bool(re.search(rf"(?<![A-Za-z0-9_]){re.escape(marker)}(?![A-Za-z0-9_])", markexpr))

def _excludes_marker(markexpr: str, marker: str) -> bool:
    return bool(re.search(rf"(?<![A-Za-z0-9_])not\s+{re.escape(marker)}(?![A-Za-z0-9_])", markexpr))


def pytest_configure(config) -> None:
    markexpr = getattr(config.option, "markexpr", "") or ""

    selects_integration = _selects_marker(markexpr, "integration")
    excludes_integration = _excludes_marker(markexpr, "integration")

    selects_functional = _selects_marker(markexpr, "functional")
    excludes_functional = _excludes_marker(markexpr, "functional")

    selects_external = (selects_integration and not excludes_integration) or (
        selects_functional and not excludes_functional
    )
    if selects_external:
        os.environ.setdefault("ARKLOOP_LOAD_DOTENV", "1")
        _prefer_test_dotenv_file()


def _prefer_test_dotenv_file() -> None:
    # 显式指定了 dotenv 文件时，保持用户选择
    if os.getenv("ARKLOOP_DOTENV_FILE"):
        return

    repo_root = _repo_root(Path(__file__).resolve())
    test_dotenv = repo_root / ".env.test"
    if test_dotenv.is_file():
        os.environ["ARKLOOP_DOTENV_FILE"] = str(test_dotenv)


def _repo_root(start: Path) -> Path:
    current = start.resolve()
    for parent in (current, *current.parents):
        if (parent / "pyproject.toml").is_file():
            return parent
    return current


def _layer_from_path(path: Path) -> str | None:
    parts = path.parts
    for i, part in enumerate(parts):
        if part == "tests" and i + 1 < len(parts):
            return parts[i + 1]
    return None


@pytest.hookimpl(tryfirst=True)
def pytest_collection_modifyitems(config, items) -> None:
    for item in items:
        path = getattr(item, "path", None)
        if path is None:
            path = Path(str(item.fspath))

        layer = _layer_from_path(Path(path))
        if layer == "integration":
            item.add_marker(pytest.mark.integration)
        elif layer == "functional":
            item.add_marker(pytest.mark.functional)
