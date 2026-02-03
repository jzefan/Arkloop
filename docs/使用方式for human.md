服务端
.\.venv\Scripts\activate.ps1
$env:ARKLOOP_LOAD_DOTENV=1
python -m uvicorn services.api.main:configure_app --factory --app-dir src --host 127.0.0.1 --port 8000

前端
pnpm -C src/apps/web dev