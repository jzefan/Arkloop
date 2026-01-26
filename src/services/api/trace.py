from __future__ import annotations

from fastapi import FastAPI

from packages.observability.context import new_trace_id
from packages.observability.http import (
    TRACE_ID_HEADER,
    TraceIdMiddlewareConfig,
    install_trace_id_middleware as _install_trace_id_middleware,
)

__all__ = ["TRACE_ID_HEADER", "install_trace_id_middleware", "new_trace_id"]


def install_trace_id_middleware(app: FastAPI, *, trust_incoming_trace_id: bool = False) -> None:
    _install_trace_id_middleware(
        app,
        config=TraceIdMiddlewareConfig(trust_incoming_trace_id=trust_incoming_trace_id),
    )
