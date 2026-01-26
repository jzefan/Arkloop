from __future__ import annotations

from typing import Awaitable, Callable
import uuid

from fastapi import FastAPI, Request
from starlette.responses import Response

TRACE_ID_HEADER = "X-Trace-Id"


def new_trace_id() -> str:
    return uuid.uuid4().hex


def install_trace_id_middleware(app: FastAPI) -> None:
    @app.middleware("http")
    async def _trace_id_middleware(
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        request.state.trace_id = new_trace_id()
        response = await call_next(request)
        response.headers[TRACE_ID_HEADER] = request.state.trace_id
        return response
