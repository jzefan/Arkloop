from __future__ import annotations

from contextlib import contextmanager
from typing import Iterator, Optional

from .context import job_context, new_trace_id, normalize_trace_id


@contextmanager
def worker_job_context(
    *,
    trace_id: Optional[str],
    org_id: Optional[str],
    run_id: Optional[str],
) -> Iterator[str]:
    chosen_trace_id = normalize_trace_id(trace_id) or new_trace_id()
    with job_context(trace_id=chosen_trace_id, org_id=org_id, run_id=run_id):
        yield chosen_trace_id


__all__ = ["worker_job_context"]

