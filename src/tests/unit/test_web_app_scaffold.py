from __future__ import annotations

import json
from pathlib import Path


def test_web_app_scaffold_exists_and_has_expected_config() -> None:
    repo_root = Path(__file__).resolve().parents[3]
    web_root = repo_root / "src" / "apps" / "web"

    package_json_path = web_root / "package.json"
    assert package_json_path.exists(), "缺少前端工程：src/apps/web/package.json"

    package_json = json.loads(package_json_path.read_text(encoding="utf-8"))

    scripts = package_json.get("scripts", {})
    assert scripts.get("dev") == "vite", "前端脚本缺少 dev=vite"
    assert "vite build" in (scripts.get("build") or ""), "前端脚本缺少 build（需包含 vite build）"

    dependencies = package_json.get("dependencies", {})
    dev_dependencies = package_json.get("devDependencies", {})

    for dep_name in ["react", "react-dom"]:
        assert dep_name in dependencies, f"缺少依赖：{dep_name}"

    for dep_name in ["vite", "typescript", "tailwindcss", "@tailwindcss/vite", "@vitejs/plugin-react"]:
        assert dep_name in dev_dependencies, f"缺少开发依赖：{dep_name}"

    pnpm_cfg = package_json.get("pnpm", {})
    only_built = pnpm_cfg.get("onlyBuiltDependencies", [])
    assert "esbuild" in only_built, "pnpm 未放行 esbuild 的构建脚本（pnpm.onlyBuiltDependencies）"

    vite_config_path = web_root / "vite.config.ts"
    vite_config = vite_config_path.read_text(encoding="utf-8")
    assert "@tailwindcss/vite" in vite_config, "vite 未启用 @tailwindcss/vite 插件"

    index_css_path = web_root / "src" / "index.css"
    index_css = index_css_path.read_text(encoding="utf-8")
    assert '@import "tailwindcss";' in index_css, "Tailwind 样式入口未正确引入（@import \"tailwindcss\";）"
