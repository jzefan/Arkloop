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


async def _seed_user(sqlalchemy_url: str, login: str, password: str) -> uuid.UUID:
    database = Database.from_config(DatabaseConfig(url=sqlalchemy_url))
    try:
        async with database.sessionmaker() as session:
            org_repo = SqlAlchemyOrgRepository(session)
            user_repo = SqlAlchemyUserRepository(session)
            membership_repo = SqlAlchemyOrgMembershipRepository(session)
            credential_repo = SqlAlchemyUserCredentialRepository(session)

            org_slug = f"org_{uuid.uuid4().hex}"
            org = await org_repo.create(slug=org_slug, name=f"Org {org_slug}")

            user = await user_repo.create(display_name="Alice")

            hasher = BcryptPasswordHasher()
            await credential_repo.create(
                user_id=user.id,
                login=login,
                password_hash=hasher.hash_password(password),
            )

            await membership_repo.create(org_id=org.id, user_id=user.id, role="owner")
            await session.commit()
            return user.id
    finally:
        await database.dispose()


def _assert_unauthorized(resp, *, expected_code: str) -> None:
    assert resp.status_code == 401
    assert TRACE_ID_HEADER in resp.headers
    assert resp.headers[TRACE_ID_HEADER]
    payload = resp.json()
    assert payload["trace_id"] == resp.headers[TRACE_ID_HEADER]
    assert payload["code"] == expected_code


def test_logout_invalidates_old_token(monkeypatch) -> None:
    config = DatabaseConfig.from_env(allow_fallback=True)
    if config is None:
        pytest.skip("未设置 ARKLOOP_DATABASE_URL（或兼容的 DATABASE_URL）")

    repo_root = _repo_root()
    alembic_cfg = Config(str(repo_root / "alembic.ini"))

    database = f"arkloop_auth_logout_{uuid.uuid4().hex}"
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

            password = "pwdpwdpwd"
            anyio.run(_seed_user, test_sqlalchemy_url, "alice", password)

            app = configure_app()
            with TestClient(app) as client:
                login_resp = client.post(
                    "/v1/auth/login",
                    json={"login": "alice", "password": password},
                )
                assert login_resp.status_code == 200
                token = login_resp.json()["access_token"]
                headers = {"Authorization": f"Bearer {token}"}

                me_resp = client.get("/v1/me", headers=headers)
                assert me_resp.status_code == 200

                logout_resp = client.post("/v1/auth/logout", headers=headers)
                assert logout_resp.status_code == 200
                assert logout_resp.json() == {"ok": True}

                me_after_logout = client.get("/v1/me", headers=headers)
                _assert_unauthorized(me_after_logout, expected_code="auth.invalid_token")

                refresh_after_logout = client.post("/v1/auth/refresh", headers=headers)
                _assert_unauthorized(refresh_after_logout, expected_code="auth.invalid_token")

                new_login = client.post(
                    "/v1/auth/login",
                    json={"login": "alice", "password": password},
                )
                assert new_login.status_code == 200
                new_token = new_login.json()["access_token"]
                new_headers = {"Authorization": f"Bearer {new_token}"}

                me_new = client.get("/v1/me", headers=new_headers)
                assert me_new.status_code == 200
    finally:
        anyio.run(_drop_database, admin_dsn, database)
