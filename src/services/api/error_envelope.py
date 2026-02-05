from __future__ import annotations

import logging
from typing import Any, Awaitable, Callable, Optional

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from pydantic import BaseModel
from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.responses import Response

from packages.observability.context import trace_id_context

from .trace import TRACE_ID_HEADER, new_trace_id


class ErrorEnvelope(BaseModel):
    code: str
    message: str
    trace_id: str
    details: Optional[Any] = None


class ApiError(Exception):
    def __init__(
        self,
        *,
        code: str,
        message: str,
        status_code: int = 400,
        details: Optional[Any] = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.status_code = status_code
        self.details = details


def _ensure_trace_id(request: Request) -> str:
    trace_id = getattr(request.state, "trace_id", None)
    if isinstance(trace_id, str) and trace_id:
        return trace_id

    trace_id = new_trace_id()
    request.state.trace_id = trace_id
    return trace_id


def _error_response(
    *,
    status_code: int,
    code: str,
    message: str,
    trace_id: str,
    details: Optional[Any] = None,
) -> JSONResponse:
    envelope = ErrorEnvelope(
        code=code,
        message=message,
        details=details,
        trace_id=trace_id,
    )
    return JSONResponse(
        status_code=status_code,
        content=envelope.model_dump(exclude_none=True),
        headers={TRACE_ID_HEADER: trace_id},
    )


def install_error_handlers(app: FastAPI) -> None:
    @app.exception_handler(ApiError)
    async def _handle_api_error(request: Request, exc: ApiError) -> JSONResponse:
        trace_id = _ensure_trace_id(request)
        return _error_response(
            status_code=exc.status_code,
            code=exc.code,
            message=exc.message,
            details=exc.details,
            trace_id=trace_id,
        )

    @app.exception_handler(RequestValidationError)
    async def _handle_validation_error(
        request: Request,
        exc: RequestValidationError,
    ) -> JSONResponse:
        trace_id = _ensure_trace_id(request)
        return _error_response(
            status_code=422,
            code="validation_error",
            message="请求参数校验失败",
            details=exc.errors(),
            trace_id=trace_id,
        )

    @app.exception_handler(StarletteHTTPException)
    async def _handle_http_exception(
        request: Request,
        exc: StarletteHTTPException,
    ) -> JSONResponse:
        trace_id = _ensure_trace_id(request)
        return _error_response(
            status_code=exc.status_code,
            code="http_error",
            message=str(exc.detail),
            trace_id=trace_id,
        )


def install_unhandled_exception_middleware(app: FastAPI) -> None:
    logger = logging.getLogger("arkloop.api")

    @app.middleware("http")
    async def _unhandled_exception_middleware(
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        try:
            return await call_next(request)
        except Exception:
            trace_id = _ensure_trace_id(request)
            with trace_id_context(trace_id):
                logger.exception(
                    "unhandled exception",
                    extra={
                        "method": request.method,
                        "path": request.url.path,
                        "query_params": dict(request.query_params),
                    },
                )
            return _error_response(
                status_code=500,
                code="internal_error",
                message="内部错误",
                trace_id=trace_id,
            )
