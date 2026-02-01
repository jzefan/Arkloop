from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Mapping
import uuid

from packages.observability.context import normalize_trace_id


@dataclass(frozen=True, slots=True)
class WorkerJobPayload:
    job_id: uuid.UUID
    job_type: str
    trace_id: str
    org_id: uuid.UUID
    run_id: uuid.UUID
    payload_json: Mapping[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json(cls, data: Mapping[str, Any]) -> "WorkerJobPayload":
        job_id = uuid.UUID(str(data.get("job_id")))
        job_type_raw = data.get("type")
        if not isinstance(job_type_raw, str) or not job_type_raw.strip():
            raise ValueError("job.type 必须为非空字符串")
        job_type = job_type_raw.strip()

        trace_id = normalize_trace_id(data.get("trace_id") if isinstance(data.get("trace_id"), str) else None)
        if trace_id is None:
            raise ValueError("job.trace_id 必须为 32 位十六进制字符串")

        org_id = uuid.UUID(str(data.get("org_id")))
        run_id = uuid.UUID(str(data.get("run_id")))

        payload = data.get("payload")
        if payload is None:
            payload_json: Mapping[str, Any] = {}
        elif isinstance(payload, Mapping):
            payload_json = payload
        else:
            raise ValueError("job.payload 必须为对象")

        return cls(
            job_id=job_id,
            job_type=job_type,
            trace_id=trace_id,
            org_id=org_id,
            run_id=run_id,
            payload_json=payload_json,
        )


__all__ = ["WorkerJobPayload"]

