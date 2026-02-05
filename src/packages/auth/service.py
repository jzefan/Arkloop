from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
import uuid

from packages.data.credentials import UserCredentialRepository
from packages.data.identity import User, UserRepository

from .password_hasher import PasswordHasher
from .tokens import JwtAccessTokenService, TokenInvalidError, VerifiedAccessToken


class InvalidCredentialsError(Exception):
    ...


@dataclass(frozen=True, slots=True)
class IssuedAccessToken:
    token: str
    user_id: uuid.UUID


class UserNotFoundError(LookupError):
    def __init__(self, *, user_id: uuid.UUID) -> None:
        super().__init__("用户不存在")
        self.user_id = user_id


class AuthService:
    def __init__(
        self,
        *,
        user_repo: UserRepository,
        credential_repo: UserCredentialRepository,
        password_hasher: PasswordHasher,
        token_service: JwtAccessTokenService,
    ) -> None:
        self._user_repo = user_repo
        self._credential_repo = credential_repo
        self._password_hasher = password_hasher
        self._token_service = token_service

    async def issue_access_token(self, *, login: str, password: str) -> IssuedAccessToken:
        credential = await self._credential_repo.get_by_login(login)
        if credential is None:
            raise InvalidCredentialsError("invalid_credentials")
        if not self._password_hasher.verify_password(password, credential.password_hash):
            raise InvalidCredentialsError("invalid_credentials")
        token = self._token_service.issue(user_id=credential.user_id)
        return IssuedAccessToken(token=token, user_id=credential.user_id)

    async def refresh_access_token(self, *, token: str) -> IssuedAccessToken:
        user = await self.authenticate_user(token=token)
        refreshed = self._token_service.issue(user_id=user.id)
        return IssuedAccessToken(token=refreshed, user_id=user.id)

    def verify_access_token(self, *, token: str) -> VerifiedAccessToken:
        return self._token_service.verify(token)

    async def get_user(self, *, user_id: uuid.UUID) -> User:
        user = await self._user_repo.get_by_id(user_id)
        if user is None:
            raise UserNotFoundError(user_id=user_id)
        return user

    async def authenticate_user(self, *, token: str) -> User:
        verified = self.verify_access_token(token=token)
        user = await self.get_user(user_id=verified.user_id)
        if verified.issued_at < user.tokens_invalid_before:
            raise TokenInvalidError("token 已失效")
        return user

    async def logout(self, *, user_id: uuid.UUID, now: datetime | None = None) -> None:
        tokens_invalid_before = datetime.now(timezone.utc) if now is None else now
        await self._user_repo.bump_tokens_invalid_before(
            user_id=user_id,
            tokens_invalid_before=tokens_invalid_before,
        )


__all__ = ["AuthService", "InvalidCredentialsError", "IssuedAccessToken", "UserNotFoundError"]
