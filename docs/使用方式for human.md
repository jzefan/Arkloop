后端
.\.venv\Scripts\activate.ps1
$env:ARKLOOP_LOAD_DOTENV=1
docker compose up -d postgres
python -m alembic upgrade head
python -m uvicorn services.api.main:configure_app --factory --app-dir src --host 127.0.0.1 --port 8000

前端
pnpm -C src/apps/web dev

# 前端Debug
$env:ARKLOOP_LLM_DEBUG_EVENTS=1 


测试
pytest
pytest -m integration

# integration 建议使用独立环境文件（避免命中真实 LLM）
# 1) 复制 .env.test.example 为 .env.test 并填写数据库配置
# 2) 直接运行 pytest -m integration（conftest 会优先加载 .env.test）

#llm发消息
python -m arkloop chat --profile llm --message "你好你是谁"
