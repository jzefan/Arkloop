from __future__ import annotations

import logging
from typing import Mapping

from packages.data import Database
from packages.data.runs import SqlAlchemyRunEventRepository
from packages.observability.worker import worker_job_context

from .job_payload import WorkerJobPayload


class Worker:
    def __init__(self, *, database: Database) -> None:
        self._database = database
        self._logger = logging.getLogger("arkloop.worker")

    async def handle_job(self, payload_json: Mapping[str, object]) -> None:
        job = WorkerJobPayload.from_json(payload_json)
        with worker_job_context(
            trace_id=job.trace_id,
            org_id=str(job.org_id),
            run_id=str(job.run_id),
        ) as trace_id:
            self._logger.info(
                "收到 job",
                extra={
                    "job_id": str(job.job_id),
                    "job_type": job.job_type,
                    "org_id": str(job.org_id),
                    "run_id": str(job.run_id),
                },
            )
            async with self._database.sessionmaker() as session:
                repo = SqlAlchemyRunEventRepository(session)
                run = await repo.get_run(run_id=job.run_id)
                if run is None:
                    raise LookupError("Run 不存在")
                if run.org_id != job.org_id:
                    raise PermissionError("job.org_id 与 run.org_id 不一致")

                await repo.append_event(
                    run_id=job.run_id,
                    type="worker.job.received",
                    data_json={
                        "trace_id": trace_id,
                        "job_id": str(job.job_id),
                        "job_type": job.job_type,
                        "org_id": str(job.org_id),
                    },
                )
                await session.commit()


__all__ = ["Worker"]

