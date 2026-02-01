from __future__ import annotations

from contextlib import contextmanager
import contextvars
import re
from typing import Iterator, Optional
import uuid

_TRACE_ID_RE = re.compile(r"^[0-9a-f]{32}$", re.IGNORECASE)
_trace_id: contextvars.ContextVar[Optional[str]] = contextvars.ContextVar("trace_id", default=None)
_org_id: contextvars.ContextVar[Optional[str]] = contextvars.ContextVar("org_id", default=None)
_run_id: contextvars.ContextVar[Optional[str]] = contextvars.ContextVar("run_id", default=None)


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


def get_org_id() -> Optional[str]:
    return _org_id.get()


def set_org_id(org_id: Optional[str]) -> contextvars.Token[Optional[str]]:
    return _org_id.set(org_id)


def reset_org_id(token: contextvars.Token[Optional[str]]) -> None:
    _org_id.reset(token)


@contextmanager
def org_id_context(org_id: Optional[str]) -> Iterator[None]:
    token = set_org_id(org_id)
    try:
        yield
    finally:
        reset_org_id(token)


def get_run_id() -> Optional[str]:
    return _run_id.get()


def set_run_id(run_id: Optional[str]) -> contextvars.Token[Optional[str]]:
    return _run_id.set(run_id)


def reset_run_id(token: contextvars.Token[Optional[str]]) -> None:
    _run_id.reset(token)


@contextmanager
def run_id_context(run_id: Optional[str]) -> Iterator[None]:
    token = set_run_id(run_id)
    try:
        yield
    finally:
        reset_run_id(token)


@contextmanager
def job_context(*, trace_id: Optional[str], org_id: Optional[str], run_id: Optional[str]) -> Iterator[None]:
    trace_token = set_trace_id(trace_id)
    org_token = set_org_id(org_id)
    run_token = set_run_id(run_id)
    try:
        yield
    finally:
        reset_run_id(run_token)
        reset_org_id(org_token)
        reset_trace_id(trace_token)
