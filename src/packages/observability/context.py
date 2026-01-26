from __future__ import annotations

from contextlib import contextmanager
import contextvars
import re
from typing import Iterator, Optional
import uuid

_TRACE_ID_RE = re.compile(r"^[0-9a-f]{32}$", re.IGNORECASE)
_trace_id: contextvars.ContextVar[Optional[str]] = contextvars.ContextVar("trace_id", default=None)


def new_trace_id() -> str:
    return uuid.uuid4().hex


def normalize_trace_id(value: Optional[str]) -> Optional[str]:
    if value is None:
        return None
    candidate = value.strip()
    if not candidate:
        return None
    if not _TRACE_ID_RE.fullmatch(candidate):
        return None
    return candidate.lower()


def get_trace_id() -> Optional[str]:
    return _trace_id.get()


def set_trace_id(trace_id: Optional[str]) -> contextvars.Token[Optional[str]]:
    return _trace_id.set(trace_id)


def reset_trace_id(token: contextvars.Token[Optional[str]]) -> None:
    _trace_id.reset(token)


@contextmanager
def trace_id_context(trace_id: Optional[str]) -> Iterator[None]:
    token = set_trace_id(trace_id)
    try:
        yield
    finally:
        reset_trace_id(token)

