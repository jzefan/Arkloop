from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from datetime import datetime
from typing import Any
import uuid

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql
from sqlalchemy.ext.asyncio import AsyncSession

_metadata = sa.MetaData()

_audit_logs = sa.Table(
    "audit_logs",
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
        nullable=True,
    ),
    sa.Column(
        "actor_user_id",
        postgresql.UUID(as_uuid=True),
        sa.ForeignKey("users.id", ondelete="SET NULL"),
        nullable=True,
    ),
    sa.Column("action", sa.Text(), nullable=False),
    sa.Column("target_type", sa.Text(), nullable=True),
    sa.Column("target_id", sa.Text(), nullable=True),
    sa.Column(
        "ts",
        sa.TIMESTAMP(timezone=True),
        nullable=False,
        server_default=sa.text("now()"),
    ),
    sa.Column("trace_id", sa.Text(), nullable=False),
    sa.Column(
        "metadata_json",
        postgresql.JSONB(astext_type=sa.Text()),
        nullable=False,
        server_default=sa.text("'{}'::jsonb"),
    ),
    sa.Index("ix_audit_logs_trace_id", "trace_id"),
    sa.Index("ix_audit_logs_org_id_ts", "org_id", "ts"),
)


@dataclass(frozen=True, slots=True)
class AuditLog:
    id: uuid.UUID
    org_id: uuid.UUID | None
    actor_user_id: uuid.UUID | None
    action: str
    target_type: str | None
    target_id: str | None
    ts: datetime
    trace_id: str
    metadata_json: Any


class AuditLogRepository(ABC):
    @abstractmethod
    async def create(
        self,
        *,
        org_id: uuid.UUID | None,
        actor_user_id: uuid.UUID | None,
        action: str,
        target_type: str | None,
        target_id: str | None,
        trace_id: str,
        metadata_json: Any,
    ) -> AuditLog: ...

    @abstractmethod
    async def list_by_trace_id(self, *, trace_id: str, limit: int = 200) -> list[AuditLog]: ...


class SqlAlchemyAuditLogRepository(AuditLogRepository):
    def __init__(self, session: AsyncSession) -> None:
        self._session = session

    async def create(
        self,
        *,
        org_id: uuid.UUID | None,
        actor_user_id: uuid.UUID | None,
        action: str,
        target_type: str | None,
        target_id: str | None,
        trace_id: str,
        metadata_json: Any,
    ) -> AuditLog:
        stmt = (
            sa.insert(_audit_logs)
            .values(
                org_id=org_id,
                actor_user_id=actor_user_id,
                action=action,
                target_type=target_type,
                target_id=target_id,
                trace_id=trace_id,
                metadata_json=metadata_json,
            )
            .returning(
                _audit_logs.c.id,
                _audit_logs.c.org_id,
                _audit_logs.c.actor_user_id,
                _audit_logs.c.action,
                _audit_logs.c.target_type,
                _audit_logs.c.target_id,
                _audit_logs.c.ts,
                _audit_logs.c.trace_id,
                _audit_logs.c.metadata_json,
            )
        )
        row = (await self._session.execute(stmt)).mappings().one()
        return AuditLog(**row)

    async def list_by_trace_id(self, *, trace_id: str, limit: int = 200) -> list[AuditLog]:
        if limit <= 0:
            raise ValueError("limit must be a positive number")
        stmt = (
            sa.select(
                _audit_logs.c.id,
                _audit_logs.c.org_id,
                _audit_logs.c.actor_user_id,
                _audit_logs.c.action,
                _audit_logs.c.target_type,
                _audit_logs.c.target_id,
                _audit_logs.c.ts,
                _audit_logs.c.trace_id,
                _audit_logs.c.metadata_json,
            )
            .where(_audit_logs.c.trace_id == trace_id)
            .order_by(_audit_logs.c.ts.asc())
            .limit(limit)
        )
        rows = (await self._session.execute(stmt)).mappings().all()
        return [AuditLog(**row) for row in rows]


__all__ = [
    "AuditLog",
    "AuditLogRepository",
    "SqlAlchemyAuditLogRepository",
]

