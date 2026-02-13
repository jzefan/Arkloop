from __future__ import annotations

from collections.abc import Callable

from fastapi import Depends, FastAPI, Request
from sqlalchemy.ext.asyncio import AsyncSession

from packages.job_queue import JobQueue, SqlAlchemyPgJobQueue

from .db import get_db_session
from .error_envelope import ApiError


def install_job_queue_factory(app: FastAPI, factory: Callable[[AsyncSession], JobQueue]) -> None:
    app.state.job_queue_factory = factory


def _get_job_queue_factory(app: FastAPI) -> Callable[[AsyncSession], JobQueue]:
    factory = getattr(app.state, "job_queue_factory", None)
    if callable(factory):
        return factory
    raise ApiError(code="job_queue.not_configured", message="JobQueue 未配置", status_code=503)


def configure_job_queue(app: FastAPI) -> None:
    install_job_queue_factory(app, lambda session: SqlAlchemyPgJobQueue(session))


def get_job_queue(
    request: Request,
    session: AsyncSession = Depends(get_db_session),
) -> JobQueue:
    factory = _get_job_queue_factory(request.app)
    return factory(session)


__all__ = [
    "configure_job_queue",
    "get_job_queue",
    "install_job_queue_factory",
]

