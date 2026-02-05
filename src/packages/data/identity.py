from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from datetime import datetime
import uuid

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql
from sqlalchemy.ext.asyncio import AsyncSession

_metadata = sa.MetaData()

_orgs = sa.Table(
    "orgs",
    _metadata,
    sa.Column(
        "id",
        postgresql.UUID(as_uuid=True),
        primary_key=True,
        server_default=sa.text("gen_random_uuid()"),
    ),
    sa.Column("slug", sa.Text(), nullable=False),
    sa.Column("name", sa.Text(), nullable=False),
    sa.Column(
        "created_at",
        sa.TIMESTAMP(timezone=True),
        nullable=False,
        server_default=sa.text("now()"),
    ),
    sa.UniqueConstraint("slug", name="uq_orgs_slug"),
)

_users = sa.Table(
    "users",
    _metadata,
    sa.Column(
        "id",
        postgresql.UUID(as_uuid=True),
        primary_key=True,
        server_default=sa.text("gen_random_uuid()"),
    ),
    sa.Column("display_name", sa.Text(), nullable=False),
    sa.Column(
        "tokens_invalid_before",
        sa.TIMESTAMP(timezone=True),
        nullable=False,
        server_default=sa.text("to_timestamp(0)"),
    ),
    sa.Column(
        "created_at",
        sa.TIMESTAMP(timezone=True),
        nullable=False,
        server_default=sa.text("now()"),
    ),
)

_org_memberships = sa.Table(
    "org_memberships",
    _metadata,
    sa.Column(
        "id", postgresql.UUID(as_uuid=True), primary_key=True, server_default=sa.text("gen_random_uuid()")
    ),
    sa.Column("org_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("orgs.id"), nullable=False),
    sa.Column("user_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("users.id"), nullable=False),
    sa.Column("role", sa.Text(), nullable=False, server_default=sa.text("'member'")),
    sa.Column(
        "created_at",
        sa.TIMESTAMP(timezone=True),
        nullable=False,
        server_default=sa.text("now()"),
    ),
    sa.UniqueConstraint("org_id", "user_id", name="uq_org_memberships_org_id_user_id"),
)


@dataclass(frozen=True, slots=True)
class Org:
    id: uuid.UUID
    slug: str
    name: str
    created_at: datetime


@dataclass(frozen=True, slots=True)
class User:
    id: uuid.UUID
    display_name: str
    tokens_invalid_before: datetime
    created_at: datetime


@dataclass(frozen=True, slots=True)
class OrgMembership:
    id: uuid.UUID
    org_id: uuid.UUID
    user_id: uuid.UUID
    role: str
    created_at: datetime


class OrgRepository(ABC):
    @abstractmethod
    async def create(self, *, slug: str, name: str) -> Org: ...

    @abstractmethod
    async def get_by_id(self, org_id: uuid.UUID) -> Org | None: ...

    @abstractmethod
    async def get_by_slug(self, slug: str) -> Org | None: ...


class UserRepository(ABC):
    @abstractmethod
    async def create(self, *, display_name: str) -> User: ...

    @abstractmethod
    async def get_by_id(self, user_id: uuid.UUID) -> User | None: ...

    @abstractmethod
    async def bump_tokens_invalid_before(self, *, user_id: uuid.UUID, tokens_invalid_before: datetime) -> None: ...


class OrgMembershipRepository(ABC):
    @abstractmethod
    async def create(self, *, org_id: uuid.UUID, user_id: uuid.UUID, role: str = "member") -> OrgMembership: ...

    @abstractmethod
    async def get_by_org_and_user(self, *, org_id: uuid.UUID, user_id: uuid.UUID) -> OrgMembership | None: ...

    @abstractmethod
    async def get_default_for_user(self, *, user_id: uuid.UUID) -> OrgMembership | None: ...


class SqlAlchemyOrgRepository(OrgRepository):
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def create(self, *, slug: str, name: str) -> Org:
        stmt = (
            sa.insert(_orgs)
            .values(slug=slug, name=name)
            .returning(_orgs.c.id, _orgs.c.slug, _orgs.c.name, _orgs.c.created_at)
        )
        row = (await self._session.execute(stmt)).mappings().one()
        return Org(**row)

    async def get_by_id(self, org_id: uuid.UUID) -> Org | None:
        stmt = sa.select(_orgs.c.id, _orgs.c.slug, _orgs.c.name, _orgs.c.created_at).where(
            _orgs.c.id == org_id
        )
        row = (await self._session.execute(stmt)).mappings().one_or_none()
        return None if row is None else Org(**row)

    async def get_by_slug(self, slug: str) -> Org | None:
        stmt = sa.select(_orgs.c.id, _orgs.c.slug, _orgs.c.name, _orgs.c.created_at).where(
            _orgs.c.slug == slug
        )
        row = (await self._session.execute(stmt)).mappings().one_or_none()
        return None if row is None else Org(**row)


class SqlAlchemyUserRepository(UserRepository):
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def create(self, *, display_name: str) -> User:
        stmt = (
            sa.insert(_users)
            .values(display_name=display_name)
            .returning(
                _users.c.id,
                _users.c.display_name,
                _users.c.tokens_invalid_before,
                _users.c.created_at,
            )
        )
        row = (await self._session.execute(stmt)).mappings().one()
        return User(**row)

    async def get_by_id(self, user_id: uuid.UUID) -> User | None:
        stmt = (
            sa.select(
                _users.c.id,
                _users.c.display_name,
                _users.c.tokens_invalid_before,
                _users.c.created_at,
            )
            .where(_users.c.id == user_id)
        )
        row = (await self._session.execute(stmt)).mappings().one_or_none()
        return None if row is None else User(**row)

    async def bump_tokens_invalid_before(self, *, user_id: uuid.UUID, tokens_invalid_before: datetime) -> None:
        stmt = (
            sa.update(_users)
            .where(_users.c.id == user_id)
            .values(
                tokens_invalid_before=sa.func.greatest(
                    _users.c.tokens_invalid_before,
                    tokens_invalid_before,
                )
            )
        )
        await self._session.execute(stmt)


class SqlAlchemyOrgMembershipRepository(OrgMembershipRepository):
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def create(self, *, org_id: uuid.UUID, user_id: uuid.UUID, role: str = "member") -> OrgMembership:
        stmt = (
            sa.insert(_org_memberships)
            .values(org_id=org_id, user_id=user_id, role=role)
            .returning(
                _org_memberships.c.id,
                _org_memberships.c.org_id,
                _org_memberships.c.user_id,
                _org_memberships.c.role,
                _org_memberships.c.created_at,
            )
        )
        row = (await self._session.execute(stmt)).mappings().one()
        return OrgMembership(**row)

    async def get_by_org_and_user(self, *, org_id: uuid.UUID, user_id: uuid.UUID) -> OrgMembership | None:
        stmt = (
            sa.select(
                _org_memberships.c.id,
                _org_memberships.c.org_id,
                _org_memberships.c.user_id,
                _org_memberships.c.role,
                _org_memberships.c.created_at,
            )
            .where(_org_memberships.c.org_id == org_id)
            .where(_org_memberships.c.user_id == user_id)
        )
        row = (await self._session.execute(stmt)).mappings().one_or_none()
        return None if row is None else OrgMembership(**row)

    async def get_default_for_user(self, *, user_id: uuid.UUID) -> OrgMembership | None:
        stmt = (
            sa.select(
                _org_memberships.c.id,
                _org_memberships.c.org_id,
                _org_memberships.c.user_id,
                _org_memberships.c.role,
                _org_memberships.c.created_at,
            )
            .where(_org_memberships.c.user_id == user_id)
            .order_by(_org_memberships.c.created_at.asc())
            .limit(1)
        )
        row = (await self._session.execute(stmt)).mappings().one_or_none()
        return None if row is None else OrgMembership(**row)


__all__ = [
    "Org",
    "OrgMembership",
    "OrgMembershipRepository",
    "OrgRepository",
    "SqlAlchemyOrgMembershipRepository",
    "SqlAlchemyOrgRepository",
    "SqlAlchemyUserRepository",
    "User",
    "UserRepository",
]
