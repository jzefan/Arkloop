from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from datetime import datetime
import uuid

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql
from sqlalchemy.ext.asyncio import AsyncSession

_metadata = sa.MetaData()

_threads = sa.Table(
    "threads",
    _metadata,
    sa.Column(
        "id",
        postgresql.UUID(as_uuid=True),
        primary_key=True,
        server_default=sa.text("gen_random_uuid()"),
    ),
    sa.Column(
        "org_id",
        postgresql.UUID(as_uuid=True),
        sa.ForeignKey("orgs.id", ondelete="CASCADE"),
        nullable=False,
    ),
    sa.Column(
        "created_by_user_id",
        postgresql.UUID(as_uuid=True),
        sa.ForeignKey("users.id", ondelete="SET NULL"),
        nullable=True,
    ),
    sa.Column("title", sa.Text(), nullable=True),
    sa.Column(
        "created_at",
        sa.TIMESTAMP(timezone=True),
        nullable=False,
        server_default=sa.text("now()"),
    ),
    sa.UniqueConstraint("id", "org_id", name="uq_threads_id_org_id"),
)

_messages = sa.Table(
    "messages",
    _metadata,
    sa.Column(
        "id",
        postgresql.UUID(as_uuid=True),
        primary_key=True,
        server_default=sa.text("gen_random_uuid()"),
    ),
    sa.Column(
        "org_id",
        postgresql.UUID(as_uuid=True),
        nullable=False,
    ),
    sa.Column("thread_id", postgresql.UUID(as_uuid=True), nullable=False),
    sa.Column("created_by_user_id", postgresql.UUID(as_uuid=True), nullable=True),
    sa.Column("role", sa.Text(), nullable=False),
    sa.Column("content", sa.Text(), nullable=False),
    sa.Column(
        "created_at",
        sa.TIMESTAMP(timezone=True),
        nullable=False,
        server_default=sa.text("now()"),
    ),
    sa.ForeignKeyConstraint(
        ["org_id"],
        ["orgs.id"],
        name="fk_messages_org_id_orgs",
        ondelete="CASCADE",
    ),
    sa.ForeignKeyConstraint(
        ["created_by_user_id"],
        ["users.id"],
        name="fk_messages_created_by_user_id_users",
        ondelete="SET NULL",
    ),
    sa.ForeignKeyConstraint(
        ["thread_id", "org_id"],
        ["threads.id", "threads.org_id"],
        name="fk_messages_thread_org",
        ondelete="CASCADE",
    ),
)


@dataclass(frozen=True, slots=True)
class Thread:
    id: uuid.UUID
    org_id: uuid.UUID
    created_by_user_id: uuid.UUID | None
    title: str | None
    created_at: datetime


@dataclass(frozen=True, slots=True)
class Message:
    id: uuid.UUID
    org_id: uuid.UUID
    thread_id: uuid.UUID
    created_by_user_id: uuid.UUID | None
    role: str
    content: str
    created_at: datetime


class ThreadNotFoundError(LookupError):
    def __init__(self, *, thread_id: uuid.UUID) -> None:
        super().__init__("Thread 不存在")
        self.thread_id = thread_id


class ThreadRepository(ABC):
    @abstractmethod
    async def create(
        self,
        *,
        org_id: uuid.UUID,
        created_by_user_id: uuid.UUID | None = None,
        title: str | None = None,
    ) -> Thread: ...

    @abstractmethod
    async def get_by_id(self, thread_id: uuid.UUID) -> Thread | None: ...


class MessageRepository(ABC):
    @abstractmethod
    async def create(
        self,
        *,
        thread_id: uuid.UUID,
        role: str,
        content: str,
        created_by_user_id: uuid.UUID | None = None,
    ) -> Message: ...


class SqlAlchemyThreadRepository(ThreadRepository):
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def create(
        self,
        *,
        org_id: uuid.UUID,
        created_by_user_id: uuid.UUID | None = None,
        title: str | None = None,
    ) -> Thread:
        stmt = (
            sa.insert(_threads)
            .values(org_id=org_id, created_by_user_id=created_by_user_id, title=title)
            .returning(
                _threads.c.id,
                _threads.c.org_id,
                _threads.c.created_by_user_id,
                _threads.c.title,
                _threads.c.created_at,
            )
        )
        row = (await self._session.execute(stmt)).mappings().one()
        return Thread(**row)

    async def get_by_id(self, thread_id: uuid.UUID) -> Thread | None:
        stmt = (
            sa.select(
                _threads.c.id,
                _threads.c.org_id,
                _threads.c.created_by_user_id,
                _threads.c.title,
                _threads.c.created_at,
            )
            .where(_threads.c.id == thread_id)
            .limit(1)
        )
        row = (await self._session.execute(stmt)).mappings().one_or_none()
        return None if row is None else Thread(**row)


class SqlAlchemyMessageRepository(MessageRepository):
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def create(
        self,
        *,
        thread_id: uuid.UUID,
        role: str,
        content: str,
        created_by_user_id: uuid.UUID | None = None,
    ) -> Message:
        async with self._session.begin_nested():
            thread_org_stmt = sa.select(_threads.c.org_id).where(_threads.c.id == thread_id).limit(1)
            thread_row = (await self._session.execute(thread_org_stmt)).one_or_none()
            if thread_row is None:
                raise ThreadNotFoundError(thread_id=thread_id)

            org_id = thread_row[0]
            stmt = (
                sa.insert(_messages)
                .values(
                    org_id=org_id,
                    thread_id=thread_id,
                    created_by_user_id=created_by_user_id,
                    role=role,
                    content=content,
                )
                .returning(
                    _messages.c.id,
                    _messages.c.org_id,
                    _messages.c.thread_id,
                    _messages.c.created_by_user_id,
                    _messages.c.role,
                    _messages.c.content,
                    _messages.c.created_at,
                )
            )
            row = (await self._session.execute(stmt)).mappings().one()
            return Message(**row)


__all__ = [
    "Message",
    "MessageRepository",
    "SqlAlchemyMessageRepository",
    "SqlAlchemyThreadRepository",
    "ThreadNotFoundError",
    "Thread",
    "ThreadRepository",
]
