from __future__ import annotations

from dataclasses import dataclass
import uuid

from .error_envelope import ApiError


@dataclass(frozen=True, slots=True)
class Actor:
    org_id: uuid.UUID
    user_id: uuid.UUID
    org_role: str


class Authorizer:
    async def authorize(
        self,
        action: str,
        *,
        actor: Actor,
        resource_org_id: uuid.UUID,
        resource_owner_user_id: uuid.UUID | None,
    ) -> None:
        if actor.org_id != resource_org_id:
            raise ApiError(code="policy.denied", message="无权限", status_code=403, details={"action": action})

        if resource_owner_user_id is None:
            raise ApiError(code="policy.denied", message="无权限", status_code=403, details={"action": action})
        if resource_owner_user_id != actor.user_id:
            raise ApiError(code="policy.denied", message="无权限", status_code=403, details={"action": action})


__all__ = ["Actor", "Authorizer"]
