from __future__ import annotations

import uuid

from packages.agent_core import AgentRunContext, AgentRunner
from packages.data import Database
from packages.data.runs import RunNotFoundError, SqlAlchemyRunEventRepository


class RunEngine:
    def __init__(self, *, database: Database, runner: AgentRunner) -> None:
        self._database = database
        self._runner = runner

    async def execute(self, *, run_id: uuid.UUID, trace_id: str) -> None:
        async with self._database.sessionmaker() as session:
            repo = SqlAlchemyRunEventRepository(session)
            run = await repo.get_run(run_id=run_id)
            if run is None:
                raise RunNotFoundError(run_id=run_id)

            started_event = await repo.list_events(run_id=run_id, after_seq=0, limit=1)
            input_json: dict[str, object] = {
                "org_id": str(run.org_id),
                "thread_id": str(run.thread_id),
            }
            if started_event:
                data_json = started_event[0].data_json
                if isinstance(data_json, dict):
                    route_id = data_json.get("route_id")
                    if isinstance(route_id, str) and route_id.strip():
                        input_json["route_id"] = route_id.strip()

            context = AgentRunContext(run_id=run_id, trace_id=trace_id, input_json=input_json)
            async for event in self._runner.run(context=context):
                await repo.append_event(
                    run_id=run_id,
                    type=event.type,
                    data_json=event.data_json,
                    tool_name=event.tool_name,
                    error_class=event.error_class,
                )
            await session.commit()


__all__ = ["RunEngine"]
