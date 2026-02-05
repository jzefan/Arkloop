from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
import uuid

import jwt

_ALGORITHM = "HS256"
_ACCESS_TOKEN_TYPE = "access"


class TokenError(Exception):
    ...


class TokenExpiredError(TokenError):
    ...


class TokenInvalidError(TokenError):
    ...


@dataclass(frozen=True, slots=True)
class VerifiedAccessToken:
    user_id: uuid.UUID
    issued_at: datetime


class JwtAccessTokenService:
    def __init__(self, *, secret: str, ttl_seconds: int) -> None:
        if not secret:
            raise ValueError("secret 不能为空")
        if ttl_seconds <= 0:
            raise ValueError("ttl_seconds 必须为正数")
        self._secret = secret
        self._ttl_seconds = ttl_seconds

    def issue(self, *, user_id: uuid.UUID, now: datetime | None = None) -> str:
        issued_at = datetime.now(timezone.utc) if now is None else now
        expires_at = issued_at + timedelta(seconds=self._ttl_seconds)
        payload = {
            "sub": str(user_id),
            "typ": _ACCESS_TOKEN_TYPE,
            # 用浮点秒保留子秒精度，避免 logout 与新签发 token 落在同一秒时误判失效
            "iat": issued_at.timestamp(),
            "exp": int(expires_at.timestamp()),
        }
        token = jwt.encode(payload, self._secret, algorithm=_ALGORITHM)
        if isinstance(token, bytes):
            return token.decode("utf-8")
        return str(token)

    def verify(self, token: str) -> VerifiedAccessToken:
        try:
            payload = jwt.decode(
                token,
                self._secret,
                algorithms=[_ALGORITHM],
                options={"require": ["sub", "exp", "iat", "typ"]},
            )
        except jwt.ExpiredSignatureError as exc:
            raise TokenExpiredError("token 已过期") from exc
        except jwt.PyJWTError as exc:
            raise TokenInvalidError("token 无效") from exc

        if payload.get("typ") != _ACCESS_TOKEN_TYPE:
            raise TokenInvalidError("token 类型不正确")

        sub = payload.get("sub")
        try:
            user_id = uuid.UUID(str(sub))
        except (ValueError, TypeError) as exc:
            raise TokenInvalidError("token subject 无效") from exc

        iat = payload.get("iat")
        try:
            issued_at = datetime.fromtimestamp(float(iat), tz=timezone.utc)
        except (TypeError, ValueError, OSError) as exc:
            raise TokenInvalidError("token iat 无效") from exc

        return VerifiedAccessToken(user_id=user_id, issued_at=issued_at)


__all__ = [
    "JwtAccessTokenService",
    "TokenError",
    "TokenExpiredError",
    "TokenInvalidError",
    "VerifiedAccessToken",
]
