from __future__ import annotations

from datetime import datetime
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


async def _seed_users(sqlalchemy_url: str, password: str) -> tuple[uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID]:
    database = Database.from_config(DatabaseConfig(url=sqlalchemy_url))
    try:
        async with database.sessionmaker() as session:
            org_repo = SqlAlchemyOrgRepository(session)
            user_repo = SqlAlchemyUserRepository(session)
            membership_repo = SqlAlchemyOrgMembershipRepository(session)
            credential_repo = SqlAlchemyUserCredentialRepository(session)

            shared_slug = f"org_{uuid.uuid4().hex}"
            shared_org = await org_repo.create(slug=shared_slug, name=f"Org {shared_slug}")
            other_slug = f"org_{uuid.uuid4().hex}"
            other_org = await org_repo.create(slug=other_slug, name=f"Org {other_slug}")

            alice = await user_repo.create(display_name="Alice")
            bob = await user_repo.create(display_name="Bob")
            charlie = await user_repo.create(display_name="Charlie")

            hasher = BcryptPasswordHasher()
            await credential_repo.create(
                user_id=alice.id,
                login="alice",
                password_hash=hasher.hash_password(password),
            )
            await credential_repo.create(
                user_id=bob.id,
                login="bob",
                password_hash=hasher.hash_password(password),
            )
            await credential_repo.create(
                user_id=charlie.id,
                login="charlie",
                password_hash=hasher.hash_password(password),
            )

            await membership_repo.create(org_id=shared_org.id, user_id=alice.id, role="member")
            await membership_repo.create(org_id=shared_org.id, user_id=bob.id, role="member")
            await membership_repo.create(org_id=other_org.id, user_id=charlie.id, role="member")
            await session.commit()
            return shared_org.id, alice.id, bob.id, charlie.id
    finally:
        await database.dispose()


def _parse_created_at(value: str) -> datetime:
    cleaned = value.replace("Z", "+00:00")
    return datetime.fromisoformat(cleaned)


def _thread_sort_key(item: dict[str, str]) -> tuple[datetime, int]:
    created_at = _parse_created_at(item["created_at"])
    thread_id = uuid.UUID(item["id"])
    return created_at, thread_id.int


def _assert_policy_denied(resp) -> None:
    assert resp.status_code == 403
    trace_id = resp.headers.get(TRACE_ID_HEADER)
    assert trace_id
    payload = resp.json()
    assert payload["code"] == "policy.denied"
    assert payload["trace_id"] == trace_id


def test_threads_list_get_and_patch_title(monkeypatch) -> None:
    config = DatabaseConfig.from_env(allow_fallback=True)
    if config is None:
        pytest.skip("未设置 ARKLOOP_DATABASE_URL（或兼容的 DATABASE_URL）")

    repo_root = _repo_root()
    alembic_cfg = Config(str(repo_root / "alembic.ini"))

    database = f"arkloop_threads_endpoints_{uuid.uuid4().hex}"
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
            shared_org_id, alice_id, bob_id, _charlie_id = anyio.run(_seed_users, test_sqlalchemy_url, password)

            app = configure_app()
            with TestClient(app) as client:
                alice_auth = client.post("/v1/auth/login", json={"login": "alice", "password": password})
                assert alice_auth.status_code == 200
                alice_headers = {"Authorization": f"Bearer {alice_auth.json()['access_token']}"}

                bob_auth = client.post("/v1/auth/login", json={"login": "bob", "password": password})
                assert bob_auth.status_code == 200
                bob_headers = {"Authorization": f"Bearer {bob_auth.json()['access_token']}"}

                charlie_auth = client.post("/v1/auth/login", json={"login": "charlie", "password": password})
                assert charlie_auth.status_code == 200
                charlie_headers = {"Authorization": f"Bearer {charlie_auth.json()['access_token']}"}

                alice_threads: list[dict[str, str]] = []
                for title in ["a1", "a2", "a3"]:
                    resp = client.post("/v1/threads", json={"title": title}, headers=alice_headers)
                    assert resp.status_code == 201
                    alice_threads.append(resp.json())

                bob_thread = client.post("/v1/threads", json={"title": "b1"}, headers=bob_headers)
                assert bob_thread.status_code == 201
                bob_thread_id = bob_thread.json()["id"]

                charlie_thread = client.post("/v1/threads", json={"title": "c1"}, headers=charlie_headers)
                assert charlie_thread.status_code == 201
                charlie_thread_id = charlie_thread.json()["id"]

                list_resp = client.get("/v1/threads", params={"limit": 200}, headers=alice_headers)
                assert list_resp.status_code == 200
                items = list_resp.json()
                assert {item["id"] for item in items} == {t["id"] for t in alice_threads}
                assert bob_thread_id not in {item["id"] for item in items}
                assert charlie_thread_id not in {item["id"] for item in items}
                assert all(item["org_id"] == str(shared_org_id) for item in items)
                assert all(item["created_by_user_id"] == str(alice_id) for item in items)

                keys = [_thread_sort_key(item) for item in items]
                assert keys == sorted(keys, reverse=True)

                first_page = client.get("/v1/threads", params={"limit": 2}, headers=alice_headers)
                assert first_page.status_code == 200
                first_items = first_page.json()
                assert len(first_items) == 2
                cursor = first_items[-1]

                second_page = client.get(
                    "/v1/threads",
                    params={
                        "limit": 200,
                        "before_created_at": cursor["created_at"],
                        "before_id": cursor["id"],
                    },
                    headers=alice_headers,
                )
                assert second_page.status_code == 200
                second_items = second_page.json()
                assert not {item["id"] for item in second_items}.intersection({item["id"] for item in first_items})

                combined = first_items + second_items
                assert {item["id"] for item in combined} == {t["id"] for t in alice_threads}

                target_thread_id = alice_threads[0]["id"]
                patch_resp = client.patch(
                    f"/v1/threads/{target_thread_id}",
                    json={"title": "新标题"},
                    headers=alice_headers,
                )
                assert patch_resp.status_code == 200
                assert patch_resp.json()["title"] == "新标题"

                get_resp = client.get(f"/v1/threads/{target_thread_id}", headers=alice_headers)
                assert get_resp.status_code == 200
                assert get_resp.json()["title"] == "新标题"

                denied_patch = client.patch(
                    f"/v1/threads/{target_thread_id}",
                    json={"title": "非法更新"},
                    headers=bob_headers,
                )
                _assert_policy_denied(denied_patch)

                denied_get = client.get(f"/v1/threads/{target_thread_id}", headers=bob_headers)
                _assert_policy_denied(denied_get)
    finally:
        anyio.run(_drop_database, admin_dsn, database)
