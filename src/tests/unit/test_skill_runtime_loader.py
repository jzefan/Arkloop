from __future__ import annotations

from packages.skill_runtime import load_skill_registry


def test_load_skill_registry_parses_yaml_and_prompt(tmp_path) -> None:
    skill_dir = tmp_path / "demo"
    skill_dir.mkdir()
    (skill_dir / "skill.yaml").write_text(
        "\n".join(
            [
                "id: demo_skill",
                'version: "1"',
                "title: Demo Skill",
                "tool_allowlist: [echo, noop]",
                "budgets:",
                "  max_iterations: 3",
                "  max_output_tokens: 64",
                "  tool_timeout_ms: 500",
                "  tool_budget:",
                "    max_cost_micros: 123",
                "",
            ]
        ),
        encoding="utf-8",
    )
    (skill_dir / "prompt.md").write_text("PROMPT SENTINEL\n", encoding="utf-8")

    registry = load_skill_registry(tmp_path)
    skill = registry.get("demo_skill")
    assert skill is not None
    assert skill.id == "demo_skill"
    assert skill.version == "1"
    assert skill.title == "Demo Skill"
    assert skill.prompt_md == "PROMPT SENTINEL"
    assert skill.tool_allowlist == ("echo", "noop")
    assert skill.budgets.max_iterations == 3
    assert skill.budgets.max_output_tokens == 64
    assert skill.budgets.tool_timeout_ms == 500
    assert skill.budgets.tool_budget == {"max_cost_micros": 123}


def test_load_skill_registry_rejects_duplicate_ids(tmp_path) -> None:
    first = tmp_path / "a"
    first.mkdir()
    (first / "skill.yaml").write_text('id: dup\nversion: "1"\ntitle: One\n', encoding="utf-8")
    (first / "prompt.md").write_text("one", encoding="utf-8")

    second = tmp_path / "b"
    second.mkdir()
    (second / "skill.yaml").write_text('id: dup\nversion: "1"\ntitle: Two\n', encoding="utf-8")
    (second / "prompt.md").write_text("two", encoding="utf-8")

    try:
        load_skill_registry(tmp_path)
        raise AssertionError("expected ValueError")
    except ValueError as exc:
        assert "skill.id 重复" in str(exc)

