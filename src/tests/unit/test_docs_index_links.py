from __future__ import annotations

from pathlib import Path


def test_docs_readme_links_point_to_existing_files() -> None:
    repo_root = Path(__file__).resolve().parents[3]
    docs_readme = repo_root / "src" / "docs" / "README.zh-CN.md"
    readme_text = docs_readme.read_text(encoding="utf-8")

    expected_paths = [
        "src/docs/specs/api-and-sse.zh-CN.md",
        "src/docs/specs/logging-and-observability.zh-CN.md",
        "src/docs/specs/database-architecture.zh-CN.md",
        "src/docs/specs/testing-and-pytest.zh-CN.md",
        "src/docs/guides/skills-and-tools.zh-CN.md",
        "src/docs/roadmap/development-roadmap.zh-CN.md",
    ]

    missing = []
    for relative_path in expected_paths:
        if relative_path not in readme_text:
            missing.append(f"README 未引用：{relative_path}")
            continue

        if not (repo_root / relative_path).exists():
            missing.append(f"文件不存在：{relative_path}")

    assert not missing, "\n".join(missing)
