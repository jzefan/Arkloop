from __future__ import annotations

from sqlalchemy.ext.asyncio import AsyncSession

from packages.job_queue import JobQueue, SqlAlchemyPgJobQueue


def get_job_queue(session: AsyncSession) -> JobQueue:
    return SqlAlchemyPgJobQueue(session)


__all__ = ["get_job_queue"]

