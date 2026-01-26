from __future__ import annotations

import os
import re


def pytest_configure(config) -> None:
    markexpr = getattr(config.option, "markexpr", "") or ""
    selects_integration = bool(re.search(r"(?<![A-Za-z0-9_])integration(?![A-Za-z0-9_])", markexpr))
    excludes_integration = bool(re.search(r"(?<![A-Za-z0-9_])not\\s+integration(?![A-Za-z0-9_])", markexpr))

    if selects_integration and not excludes_integration:
        os.environ.setdefault("ARKLOOP_LOAD_DOTENV", "1")

