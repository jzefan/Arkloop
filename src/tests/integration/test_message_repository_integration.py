from __future__ import annotations

from pathlib import Path
import re
from urllib.parse import urlsplit, urlunsplit
import uuid

from alembic import command
from alembic.config import Config
import anyio
import asyncpg
import pytest
import sqlalchemy as sa
from sqlalchemy.exc import IntegrityError

from packages.data import Database, DatabaseConfig
from packages.data.identity import SqlAlchemyOrgRepository, SqlAlchemyUserRepository
from packages.data.threads import SqlAlchemyMessageRepository, SqlAlchemyThreadRepository

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


async def _roundtrip(sqlalchemy_url: str) -> None:
    database = Database.from_config(DatabaseConfig(url=sqlalchemy_url))
    try:
        async with database.sessionmaker() as session:
            org_repo = SqlAlchemyOrgRepository(session)
            user_repo = SqlAlchemyUserRepository(session)
            thread_repo = SqlAlchemyThreadRepository(session)
            message_repo = SqlAlchemyMessageRepository(session)

            slug = f"org_{uuid.uuid4().hex}"
            org = await org_repo.create(slug=slug, name=f"Org {slug}")
            other_slug = f"org_{uuid.uuid4().hex}"
            other_org = await org_repo.create(slug=other_slug, name=f"Org {other_slug}")
            user = await user_repo.create(display_name="Alice")
            thread = await thread_repo.create(org_id=org.id, created_by_user_id=user.id, title="t")

            message = await message_repo.create(
                org_id=org.id,
                thread_id=thread.id,
                role="user",
                content="hello",
                created_by_user_id=user.id,
            )
            await session.commit()

            assert message.org_id == org.id
            assert message.thread_id == thread.id
            assert message.created_by_user_id == user.id

            idx_stmt = sa.text(
                "SELECT 1 FROM pg_indexes "
                "WHERE schemaname = 'public' "
                "AND tablename = 'messages' "
                "AND indexname = :indexname"
            )
            idx = (await session.execute(idx_stmt, {"indexname": "ix_messages_org_id_thread_id_created_at"})).scalar_one()
            assert idx == 1

            with pytest.raises(IntegrityError):
                async with session.begin_nested():
                    await session.execute(
                        sa.text(
                            "INSERT INTO messages (org_id, thread_id, role, content) "
                            "VALUES (:org_id, :thread_id, :role, :content)"
                        ),
                        {
                            "org_id": other_org.id,
                            "thread_id": thread.id,
                            "role": "user",
                            "content": "oops",
                        },
                    )
    finally:
        await database.dispose()


def test_messages_org_consistency_is_enforced(monkeypatch) -> None:
    config = DatabaseConfig.from_env(allow_fallback=True)
    if config is None:
        pytest.skip("未设置 ARKLOOP_DATABASE_URL（或兼容的 DATABASE_URL）")

    repo_root = _repo_root()
    alembic_cfg = Config(str(repo_root / "alembic.ini"))

    database = f"arkloop_messages_{uuid.uuid4().hex}"
    sqlalchemy_url = config.url
    admin_dsn = _replace_database(_to_asyncpg_dsn(sqlalchemy_url), "postgres")
    test_sqlalchemy_url = _replace_database(sqlalchemy_url, database)

    anyio.run(_create_database, admin_dsn, database)
    try:
        with monkeypatch.context() as m:
            m.setenv("DATABASE_URL", test_sqlalchemy_url)
            command.upgrade(alembic_cfg, "head")

        anyio.run(_roundtrip, test_sqlalchemy_url)
    finally:
        anyio.run(_drop_database, admin_dsn, database)
