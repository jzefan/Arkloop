from __future__ import annotations

import logging
from typing import Callable, Optional

from .context import get_trace_id

_INSTALLED = False
_ORIGINAL_FACTORY: Optional[Callable[..., logging.LogRecord]] = None


def install_trace_log_record_factory(*, component: Optional[str] = None) -> None:
    global _INSTALLED
    global _ORIGINAL_FACTORY
    if _INSTALLED:
        return

    _ORIGINAL_FACTORY = logging.getLogRecordFactory()

    def _record_factory(*args, **kwargs) -> logging.LogRecord:
        if _ORIGINAL_FACTORY is None:
            raise RuntimeError("LogRecordFactory 未初始化")
        record = _ORIGINAL_FACTORY(*args, **kwargs)
        record.trace_id = get_trace_id()
        if component is not None:
            record.component = component
        return record

    logging.setLogRecordFactory(_record_factory)
    _INSTALLED = True

