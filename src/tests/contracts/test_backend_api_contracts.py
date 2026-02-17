from __future__ import annotations

from dataclasses import dataclass
import json
import os
from pathlib import Path
import re
import socket
from urllib.parse import urlsplit, urlunsplit
import uuid

from alembic import command
from alembic.config import Config
import anyio
import asyncpg
import httpx
import pytest
import uvicorn

from packages.data import Database, DatabaseConfig
from packages.data.runs import SqlAlchemyRunEventRepository
from packages.job_queue import RUN_EXECUTE_JOB_TYPE
from packages.job_queue.protocol import JOB_PAYLOAD_VERSION_V1
from services.api.main import configure_app
from services.api.trace import TRACE_ID_HEADER

pytestmark = pytest.mark.integration


@dataclass(frozen=True, slots=True)
class _RegisteredUser:
    user_id: uuid.UUID
    access_token: str


def _pick_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _repo_root() -> Path:
    current = Path(__file__).resolve()
    for parent in current.parents:
        if (parent / "pyproject.toml").exists():
            return parent
    raise AssertionError("未找到仓库根目录（pyproject.toml）")


def _golden_job_payload_path() -> Path:
    return _repo_root() / "src/tests/contracts/golden/job-payload/run_execute.v1.json"


def _load_job_payload_golden() -> dict[str, object]:
    payload = json.loads(_golden_job_payload_path().read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise AssertionError("golden 必须为对象")
    schema = payload.get("payload")
    if not isinstance(schema, dict):
        raise AssertionError("golden.payload 必须为对象")
    return schema


def _replace_database(url: str, database: str) -> str:
    parsed = urlsplit(url)
    path = f"/{database}"
    return urlunsplit((parsed.scheme, parsed.netloc, path, parsed.query, parsed.fragment))


def _to_asyncpg_dsn(sqlalchemy_url: str) -> str:
    parsed = urlsplit(sqlalchemy_url)
    scheme = "postgresql" if parsed.scheme == "postgresql+asyncpg" else parsed.scheme
    return urlunsplit((scheme, parsed.netloc, parsed.path, parsed.query, parsed.fragment))


def _safe_identifier(name: str) -> str:
    if not re.fullmatch(r"[A-Za-z0-9_]+", name):
        raise ValueError("非法标识符")
    return f"\"{name}\""


async def _create_database(admin_dsn: str, database: str) -> None:
    conn = await asyncpg.connect(admin_dsn)
    try:
        await conn.execute(f"CREATE DATABASE {_safe_identifier(database)}")
    finally:
        await conn.close()


async def _drop_database(admin_dsn: str, database: str) -> None:
    conn = await asyncpg.connect(admin_dsn)
    try:
        ident = _safe_identifier(database)
        try:
            await conn.execute(f"DROP DATABASE {ident} WITH (FORCE)")
        except asyncpg.PostgresError:
            await conn.execute(
                "SELECT pg_terminate_backend(pid) FROM pg_stat_activity "
                "WHERE datname = $1 AND pid <> pg_backend_pid()",
                database,
            )
            await conn.execute(f"DROP DATABASE {ident}")
    finally:
        await conn.close()


async def _append_run_events(sqlalchemy_url: str, run_id: uuid.UUID) -> list[tuple[int, str]]:
    database = Database.from_config(DatabaseConfig(url=sqlalchemy_url))
    try:
        async with database.sessionmaker() as session:
            repo = SqlAlchemyRunEventRepository(session)
            delta = await repo.append_event(
                run_id=run_id,
                type="message.delta",
                data_json={"content_delta": "hi"},
            )
            done = await repo.append_event(
                run_id=run_id,
                type="run.completed",
                data_json={"reason": "contract_test"},
            )
            await session.commit()
            return [(delta.seq, delta.type), (done.seq, done.type)]
    finally:
        await database.dispose()


async def _read_enqueued_job_payload(sqlalchemy_url: str, run_id: uuid.UUID) -> dict[str, object]:
    dsn = _to_asyncpg_dsn(sqlalchemy_url)
    conn = await asyncpg.connect(dsn)
    try:
        row = await conn.fetchrow(
            "SELECT id, job_type, payload_json FROM jobs "
            "WHERE payload_json->>'run_id' = $1 "
            "ORDER BY created_at DESC LIMIT 1",
            str(run_id),
        )
        if row is None:
            raise AssertionError("未找到 enqueue 的 job 记录")
        raw_payload = row["payload_json"]
        if isinstance(raw_payload, str):
            payload = dict(json.loads(raw_payload))
        else:
            payload = dict(raw_payload)
        payload["_job_row_id"] = str(row["id"])
        payload["_job_row_type"] = str(row["job_type"])
        return payload
    finally:
        await conn.close()


def _auth_headers(token: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}"}


def _assert_error_envelope(resp, *, status_code: int, code: str) -> dict:
    assert resp.status_code == status_code
    assert TRACE_ID_HEADER in resp.headers
    assert resp.headers[TRACE_ID_HEADER]
    payload = resp.json()
    assert payload["trace_id"] == resp.headers[TRACE_ID_HEADER]
    assert payload["code"] == code
    assert isinstance(payload.get("message"), str) and payload["message"]
    return payload


async def _register(
    client: httpx.AsyncClient, *, login: str, password: str, display_name: str
) -> _RegisteredUser:
    resp = await client.post(
        "/v1/auth/register",
        json={"login": login, "password": password, "display_name": display_name},
    )
    assert resp.status_code == 201
    payload = resp.json()
    assert payload["token_type"] == "bearer"
    assert payload["access_token"]
    return _RegisteredUser(user_id=uuid.UUID(payload["user_id"]), access_token=str(payload["access_token"]))


async def _collect_sse_events(
    response: httpx.Response, *, expected: int, timeout_seconds: float = 5.0
) -> list[dict]:
    events: list[dict] = []
    buffer: list[str] = []

    def _flush_frame(lines: list[str]) -> dict | None:
        if not lines:
            return None
        data_lines: list[str] = []
        frame: dict[str, object] = {}
        for item in lines:
            if item.startswith(":"):
                return None
            if item.startswith("id:"):
                frame["id"] = item[len("id:") :].strip()
                continue
            if item.startswith("event:"):
                frame["event"] = item[len("event:") :].strip()
                continue
            if item.startswith("data:"):
                data_lines.append(item[len("data:") :].lstrip())
                continue
        if not data_lines:
            return None
        frame["data"] = json.loads("\n".join(data_lines))
        return frame

    with anyio.fail_after(timeout_seconds):
        async for line in response.aiter_lines():
            if line == "":
                frame = _flush_frame(buffer)
                buffer.clear()
                if frame is None:
                    continue
                events.append(frame)
                if len(events) >= expected:
                    break
                continue

            buffer.append(line)

    return events


async def _collect_sse_pings(
    response: httpx.Response, *, expected: int, timeout_seconds: float = 2.0
) -> list[str]:
    messages: list[str] = []

    with anyio.fail_after(timeout_seconds):
        async for line in response.aiter_lines():
            if not line or not line.startswith(":"):
                continue
            messages.append(line[1:].strip())
            if len(messages) >= expected:
                break

    return messages


@pytest.fixture()
def migrated_database_url(monkeypatch) -> str:
    # contracts 用例默认也走 .env.test，方便本地直接跑
    repo_root = _repo_root()
    dotenv_file = repo_root / ".env.test"
    monkeypatch.setenv("ARKLOOP_LOAD_DOTENV", "1")
    if dotenv_file.is_file() and not os.getenv("ARKLOOP_DOTENV_FILE"):
        monkeypatch.setenv("ARKLOOP_DOTENV_FILE", str(dotenv_file))

    config = DatabaseConfig.from_env(allow_fallback=True)
    if config is None:
        pytest.skip("未设置 ARKLOOP_DATABASE_URL（或兼容的 DATABASE_URL）")

    alembic_cfg = Config(str(repo_root / "alembic.ini"))

    database = f"arkloop_contract_p01_{uuid.uuid4().hex}"
    sqlalchemy_url = config.url
    admin_dsn = _replace_database(_to_asyncpg_dsn(sqlalchemy_url), "postgres")
    test_sqlalchemy_url = _replace_database(sqlalchemy_url, database)

    anyio.run(_create_database, admin_dsn, database)
    try:
        with monkeypatch.context() as m:
            m.setenv("DATABASE_URL", test_sqlalchemy_url)
            command.upgrade(alembic_cfg, "head")
        yield test_sqlalchemy_url
    finally:
        anyio.run(_drop_database, admin_dsn, database)


async def _wait_for_server_started(server: uvicorn.Server, *, timeout_seconds: float = 5.0) -> None:
    with anyio.fail_after(timeout_seconds):
        while not server.started:
            await anyio.sleep(0.01)


async def _exercise_contract(*, base_url: str, sqlalchemy_url: str) -> None:
    async with httpx.AsyncClient(base_url=base_url, timeout=5.0) as client:
        user = await _register(client, login="alice", password="pwdpwdpwd", display_name="Alice")

        # 409：重复注册
        dup = await client.post(
            "/v1/auth/register",
            json={"login": "alice", "password": "pwdpwdpwd", "display_name": "Dup"},
        )
        _assert_error_envelope(dup, status_code=409, code="auth.login_exists")

        # 401：缺少 token
        missing = await client.get("/v1/me")
        _assert_error_envelope(missing, status_code=401, code="auth.missing_token")

        me = await client.get("/v1/me", headers=_auth_headers(user.access_token))
        assert me.status_code == 200
        assert TRACE_ID_HEADER in me.headers
        assert me.headers[TRACE_ID_HEADER]
        me_payload = me.json()
        assert uuid.UUID(me_payload["id"]) == user.user_id
        assert me_payload["display_name"] == "Alice"
        assert me_payload["created_at"]

        login = await client.post("/v1/auth/login", json={"login": "alice", "password": "pwdpwdpwd"})
        assert login.status_code == 200
        login_payload = login.json()
        assert login_payload["token_type"] == "bearer"
        assert login_payload["access_token"]

        refresh = await client.post(
            "/v1/auth/refresh", headers=_auth_headers(login_payload["access_token"])
        )
        assert refresh.status_code == 200
        refreshed = refresh.json()["access_token"]
        assert refreshed

        logout = await client.post("/v1/auth/logout", headers=_auth_headers(refreshed))
        assert logout.status_code == 200
        assert logout.json() == {"ok": True}

        me_after_logout = await client.get("/v1/me", headers=_auth_headers(refreshed))
        _assert_error_envelope(me_after_logout, status_code=401, code="auth.invalid_token")

        relogin = await client.post("/v1/auth/login", json={"login": "alice", "password": "pwdpwdpwd"})
        assert relogin.status_code == 200
        headers = _auth_headers(relogin.json()["access_token"])

        # threads/messages/runs 最小闭环
        thread = await client.post("/v1/threads", json={"title": "t"}, headers=headers)
        assert thread.status_code == 201
        thread_id = uuid.UUID(thread.json()["id"])

        # 422：游标不完整
        cursor_incomplete = await client.get(f"/v1/threads?before_id={thread_id}", headers=headers)
        details = _assert_error_envelope(
            cursor_incomplete, status_code=422, code="validation_error"
        )["details"]
        assert isinstance(details, dict)
        assert details.get("reason") == "cursor_incomplete"

        # 404：资源不存在
        missing_thread = await client.get(f"/v1/threads/{uuid.uuid4()}", headers=headers)
        _assert_error_envelope(missing_thread, status_code=404, code="threads.not_found")

        # 403：跨 org 访问拒绝
        other = await _register(client, login="bob", password="pwdpwdpwd", display_name="Bob")
        forbidden = await client.get(f"/v1/threads/{thread_id}", headers=_auth_headers(other.access_token))
        _assert_error_envelope(forbidden, status_code=403, code="policy.denied")

        message = await client.post(
            f"/v1/threads/{thread_id}/messages",
            json={"content": "hello"},
            headers=headers,
        )
        assert message.status_code == 201
        assert uuid.UUID(message.json()["thread_id"]) == thread_id
        assert message.json()["role"] == "user"

        run_resp = await client.post(f"/v1/threads/{thread_id}/runs", headers=headers)
        assert run_resp.status_code == 201
        run_payload = run_resp.json()
        run_id = uuid.UUID(run_payload["run_id"])
        assert run_payload["trace_id"] == run_resp.headers[TRACE_ID_HEADER]

        # jobs.payload_json 契约（API enqueue -> Worker 解析）
        job_payload = await _read_enqueued_job_payload(sqlalchemy_url, run_id)
        golden = _load_job_payload_golden()

        assert job_payload.get("_job_row_type") == RUN_EXECUTE_JOB_TYPE
        assert job_payload.get("v") == golden["v"] == JOB_PAYLOAD_VERSION_V1
        assert job_payload.get("type") == golden["type"] == RUN_EXECUTE_JOB_TYPE
        assert job_payload.get("run_id") == str(run_id)
        assert job_payload.get("job_id") == job_payload.get("_job_row_id")
        assert job_payload.get("payload", {}).get("source") == "api"

        # SSE：断线重连（after_seq）与事件 envelope
        await _append_run_events(sqlalchemy_url, run_id)

        async with client.stream(
            "GET",
            f"/v1/runs/{run_id}/events?after_seq=0&follow=0",
            headers=headers,
        ) as resp:
            assert resp.status_code == 200
            assert resp.headers["content-type"].startswith("text/event-stream")
            assert resp.headers.get("Cache-Control") == "no-cache"
            assert resp.headers.get("X-Accel-Buffering") == "no"
            assert TRACE_ID_HEADER in resp.headers
            frames = await _collect_sse_events(resp, expected=3)

        seqs = [int(frame["data"]["seq"]) for frame in frames]
        assert seqs == sorted(seqs)
        for frame in frames:
            data = frame["data"]
            assert frame.get("id") == str(data["seq"])
            assert frame.get("event") == data["type"]
            assert data["run_id"] == str(run_id)
            assert data["event_id"]
            assert data["ts"]
            assert "data" in data

        async with client.stream(
            "GET",
            f"/v1/runs/{run_id}/events?after_seq=1&follow=0",
            headers=headers,
        ) as resp:
            assert resp.status_code == 200
            resumed = await _collect_sse_events(resp, expected=2)
        assert [frame["data"]["seq"] for frame in resumed] == seqs[1:]

        # SSE：follow=true 心跳（至少两次 ping）
        last_seq = seqs[-1]
        async with client.stream(
            "GET",
            f"/v1/runs/{run_id}/events?after_seq={last_seq}&follow=1",
            headers=headers,
        ) as resp:
            assert resp.status_code == 200
            pings = await _collect_sse_pings(resp, expected=2)
        assert pings == ["ping", "ping"]


def test_p01_backend_http_sse_and_job_payload_contract(monkeypatch, migrated_database_url: str) -> None:
    monkeypatch.setenv("DATABASE_URL", migrated_database_url)
    monkeypatch.setenv("ARKLOOP_DATABASE_URL", migrated_database_url)
    monkeypatch.setenv("ARKLOOP_AUTH_JWT_SECRET", "test-secret-should-be-long-enough-32chars")
    monkeypatch.setenv("ARKLOOP_RUN_EXECUTOR", "worker")
    monkeypatch.setenv("ARKLOOP_SSE_POLL_SECONDS", "0.01")
    monkeypatch.setenv("ARKLOOP_SSE_HEARTBEAT_SECONDS", "0.05")

    app = configure_app()

    async def _run() -> None:
        port = _pick_free_port()
        config = uvicorn.Config(
            app,
            host="127.0.0.1",
            port=port,
            log_level="warning",
            access_log=False,
        )
        server = uvicorn.Server(config)
        async with anyio.create_task_group() as tg:
            tg.start_soon(server.serve)
            await _wait_for_server_started(server)
            try:
                await _exercise_contract(
                    base_url=f"http://127.0.0.1:{port}",
                    sqlalchemy_url=migrated_database_url,
                )
            finally:
                server.should_exit = True

    anyio.run(_run)
