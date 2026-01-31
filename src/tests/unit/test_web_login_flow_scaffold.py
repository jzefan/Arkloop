from __future__ import annotations

from pathlib import Path


def test_web_login_flow_uses_in_memory_token_and_surfaces_trace_id() -> None:
    repo_root = Path(__file__).resolve().parents[3]
    web_src = repo_root / "src" / "apps" / "web" / "src"
    assert web_src.exists(), "缺少前端源码目录：src/apps/web/src"

    storage_ts = web_src / "storage.ts"
    assert storage_ts.exists(), "缺少 storage 封装：src/apps/web/src/storage.ts"

    code_files = sorted(
        p
        for p in web_src.rglob("*")
        if p.is_file() and p.suffix in {".ts", ".tsx", ".js", ".jsx"}
    )
    assert code_files, "未找到前端源码文件"

    joined = "\n".join(p.read_text(encoding="utf-8") for p in code_files)

    assert "/v1/auth/login" in joined, "前端未调用 /v1/auth/login"
    assert "/v1/me" in joined, "前端未调用 /v1/me"

    assert "trace_id" in joined, "失败时未展示 trace_id"

    assert "sessionStorage" not in joined, "不应使用 sessionStorage 保存任何状态"

    storage_code = storage_ts.read_text(encoding="utf-8")
    for marker in ["access_token", "refresh_token", "id_token", "Authorization", "Bearer"]:
        assert marker not in storage_code, f"storage.ts 不应处理 token（发现 {marker}）"

    for path in code_files:
        if path == storage_ts:
            continue
        code = path.read_text(encoding="utf-8")
        assert "localStorage" not in code, f"不应在 {path.relative_to(repo_root)} 直接使用 localStorage"
