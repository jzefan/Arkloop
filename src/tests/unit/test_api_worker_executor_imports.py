from __future__ import annotations

import os
from pathlib import Path
import subprocess
import sys
import textwrap


def _repo_root() -> Path:
    current = Path(__file__).resolve()
    for parent in current.parents:
        if (parent / "pyproject.toml").is_file():
            return parent
    raise AssertionError("未找到仓库根目录（pyproject.toml）")


def test_api_worker_executor_does_not_import_agent_loop_or_llm_gateway() -> None:
    repo_root = _repo_root()
    src_path = repo_root / "src"

    script = textwrap.dedent(
        f"""
        import os
        import sys

        sys.path.insert(0, {str(src_path)!r})

        os.environ["ARKLOOP_RUN_EXECUTOR"] = "worker"
        os.environ["ARKLOOP_DATABASE_URL"] = "postgresql+asyncpg://user:pass@localhost:5432/db"

        from services.api.main import configure_app

        configure_app()

        bad = []
        if "services.api.provider_routed_runner" in sys.modules:
            bad.append("services.api.provider_routed_runner")
        if "packages.agent_core.loop" in sys.modules:
            bad.append("packages.agent_core.loop")
        if "packages.llm_gateway" in sys.modules:
            bad.append("packages.llm_gateway")
        bad.extend(name for name in sys.modules if name.startswith("packages.llm_gateway."))

        if bad:
            raise SystemExit("bad imports: " + ",".join(sorted(set(bad))))

        print("ok")
        """
    ).strip()

    env = os.environ.copy()
    result = subprocess.run(
        [sys.executable, "-c", script],
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr or result.stdout

