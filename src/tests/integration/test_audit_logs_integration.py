from __future__ import annotations

from pathlib import Path
import re
from urllib.parse import urlsplit, urlunsplit
import uuid

from alembic import command
from alembic.config import Config
import anyio
import asyncpg
from fastapi.testclient import TestClient
import pytest

from packages.auth import BcryptPasswordHasher
from packages.data import Database, DatabaseConfig
from packages.data.audit_logs import SqlAlchemyAuditLogRepository
from packages.data.credentials import SqlAlchemyUserCredentialRepository
from packages.data.identity import (
    SqlAlchemyOrgMembershipRepository,
    SqlAlchemyOrgRepository,
    SqlAlchemyUserRepository,
)
from services.api.main import configure_app
from services.api.trace import TRACE_ID_HEADER

pytestmark = pytest.mark.integration


def _repo_root() -> Path:
    current = Path(__file__).resolve()
    for parent in current.parents:
        if (parent / "pyproject.toml").exists():
            return parent
    raise AssertionError("未找到仓库根目录（pyproject.toml）")


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
    return f'"{name}"'


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


async def _seed_two_users(sqlalchemy_url: str) -> tuple[uuid.UUID, uuid.UUID, uuid.UUID]:
    database = Database.from_config(DatabaseConfig(url=sqlalchemy_url))
    try:
        async with database.sessionmaker() as session:
            org_repo = SqlAlchemyOrgRepository(session)
            user_repo = SqlAlchemyUserRepository(session)
            membership_repo = SqlAlchemyOrgMembershipRepository(session)
            credential_repo = SqlAlchemyUserCredentialRepository(session)

            slug = f"org_{uuid.uuid4().hex}"
            org = await org_repo.create(slug=slug, name=f"Org {slug}")

            hasher = BcryptPasswordHasher()

            alice = await user_repo.create(display_name="Alice")
            await credential_repo.create(
                user_id=alice.id,
                login="alice",
                password_hash=hasher.hash_password("pwdpwdpwd"),
            )
            await membership_repo.create(org_id=org.id, user_id=alice.id, role="member")

            bob = await user_repo.create(display_name="Bob")
            await credential_repo.create(
                user_id=bob.id,
                login="bob",
                password_hash=hasher.hash_password("pwdpwdpwd2"),
            )
            await membership_repo.create(org_id=org.id, user_id=bob.id, role="member")

            await session.commit()
            return org.id, alice.id, bob.id
    finally:
        await database.dispose()


async def _list_audit_logs(sqlalchemy_url: str, trace_id: str):
    database = Database.from_config(DatabaseConfig(url=sqlalchemy_url))
    try:
        async with database.sessionmaker() as session:
            repo = SqlAlchemyAuditLogRepository(session)
            return await repo.list_by_trace_id(trace_id=trace_id)
    finally:
        await database.dispose()


def test_audit_logs_records_login_and_denied(monkeypatch) -> None:
    config = DatabaseConfig.from_env(allow_fallback=True)
    if config is None:
        pytest.skip("未设置 ARKLOOP_DATABASE_URL（或兼容的 DATABASE_URL）")

    repo_root = _repo_root()
    alembic_cfg = Config(str(repo_root / "alembic.ini"))

    database = f"arkloop_audit_{uuid.uuid4().hex}"
    sqlalchemy_url = config.url
    admin_dsn = _replace_database(_to_asyncpg_dsn(sqlalchemy_url), "postgres")
    test_sqlalchemy_url = _replace_database(sqlalchemy_url, database)

    anyio.run(_create_database, admin_dsn, database)
    try:
        with monkeypatch.context() as m:
            m.setenv("DATABASE_URL", test_sqlalchemy_url)
            m.setenv("ARKLOOP_DATABASE_URL", test_sqlalchemy_url)
            m.setenv("ARKLOOP_AUTH_JWT_SECRET", "test-secret-should-be-long-enough-32chars")
            command.upgrade(alembic_cfg, "head")

            org_id, alice_id, bob_id = anyio.run(_seed_two_users, test_sqlalchemy_url)
            assert org_id
            assert alice_id
            assert bob_id

            app = configure_app()
            with TestClient(app) as client:
                bad_login = client.post(
                    "/v1/auth/login",
                    json={"login": "alice", "password": "wrong"},
                )
                assert bad_login.status_code == 401
                assert TRACE_ID_HEADER in bad_login.headers
                bad_trace_id = bad_login.headers[TRACE_ID_HEADER]
                assert bad_trace_id

                auth1 = client.post(
                    "/v1/auth/login",
                    json={"login": "alice", "password": "pwdpwdpwd"},
                )
                assert auth1.status_code == 200
                assert TRACE_ID_HEADER in auth1.headers
                ok_trace_id = auth1.headers[TRACE_ID_HEADER]
                assert ok_trace_id
                token1 = auth1.json()["access_token"]

                thread_resp = client.post(
                    "/v1/threads",
                    json={"title": "t"},
                    headers={"Authorization": f"Bearer {token1}"},
                )
                assert thread_resp.status_code == 201
                thread_id = thread_resp.json()["id"]
                assert thread_id

                auth2 = client.post(
                    "/v1/auth/login",
                    json={"login": "bob", "password": "pwdpwdpwd2"},
                )
                assert auth2.status_code == 200
                token2 = auth2.json()["access_token"]

                denied = client.get(
                    f"/v1/threads/{thread_id}/messages",
                    headers={"Authorization": f"Bearer {token2}"},
                )
                assert denied.status_code == 403
                assert TRACE_ID_HEADER in denied.headers
                denied_trace_id = denied.headers[TRACE_ID_HEADER]
                assert denied_trace_id

            bad_logs = anyio.run(_list_audit_logs, test_sqlalchemy_url, bad_trace_id)
            assert any(
                log.action == "auth.login" and log.metadata_json.get("result") == "failed"
                for log in bad_logs
            )
            assert all(log.trace_id == bad_trace_id for log in bad_logs)

            ok_logs = anyio.run(_list_audit_logs, test_sqlalchemy_url, ok_trace_id)
            ok_login = next(
                (log for log in ok_logs if log.action == "auth.login" and log.metadata_json.get("result") == "succeeded"),
                None,
            )
            assert ok_login is not None
            assert ok_login.org_id == org_id
            assert ok_login.actor_user_id == alice_id
            assert ok_login.target_type == "user"
            assert ok_login.target_id == str(alice_id)
            assert ok_login.trace_id == ok_trace_id

            denied_logs = anyio.run(_list_audit_logs, test_sqlalchemy_url, denied_trace_id)
            denied_access = next(
                (log for log in denied_logs if log.action == "messages.list" and log.metadata_json.get("result") == "denied"),
                None,
            )
            assert denied_access is not None
            assert denied_access.org_id == org_id
            assert denied_access.actor_user_id == bob_id
            assert denied_access.target_type == "thread"
            assert denied_access.target_id == thread_id
            assert denied_access.trace_id == denied_trace_id
    finally:
        anyio.run(_drop_database, admin_dsn, database)

