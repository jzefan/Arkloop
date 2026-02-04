from __future__ import annotations

from datetime import datetime, timedelta, timezone
import uuid

import anyio
from fastapi.testclient import TestClient

from packages.data.credentials import UserCredential
from packages.data.identity import User
from services.api.main import configure_app
from services.api.trace import TRACE_ID_HEADER
import services.api.v1 as api_v1


class InMemoryUserRepository:
    def __init__(self) -> None:
        self._users: dict[uuid.UUID, User] = {}

    async def create(self, *, display_name: str) -> User:
        user = User(id=uuid.uuid4(), display_name=display_name, created_at=datetime.now(timezone.utc))
        self._users[user.id] = user
        return user

    async def get_by_id(self, user_id: uuid.UUID) -> User | None:
        return self._users.get(user_id)


class InMemoryUserCredentialRepository:
    def __init__(self) -> None:
        self._by_id: dict[uuid.UUID, UserCredential] = {}
        self._by_login: dict[str, UserCredential] = {}
        self._by_user_id: dict[uuid.UUID, UserCredential] = {}

    async def create(self, *, user_id: uuid.UUID, login: str, password_hash: str) -> UserCredential:
        credential = UserCredential(
            id=uuid.uuid4(),
            user_id=user_id,
            login=login,
            password_hash=password_hash,
            created_at=datetime.now(timezone.utc),
        )
        self._by_id[credential.id] = credential
        self._by_login[credential.login] = credential
        self._by_user_id[credential.user_id] = credential
        return credential

    async def get_by_login(self, login: str) -> UserCredential | None:
        return self._by_login.get(login)

    async def get_by_user_id(self, user_id: uuid.UUID) -> UserCredential | None:
        return self._by_user_id.get(user_id)


def _assert_auth_error_has_trace_id(response, *, expected_code: str) -> None:
    assert response.status_code == 401
    assert TRACE_ID_HEADER in response.headers
    assert response.headers[TRACE_ID_HEADER]
    payload = response.json()
    assert payload["trace_id"] == response.headers[TRACE_ID_HEADER]
    assert payload["code"] == expected_code


def test_refresh_returns_new_token_and_me_works(monkeypatch) -> None:
    monkeypatch.setenv("ARKLOOP_AUTH_JWT_SECRET", "test-secret-should-be-long-enough-32chars")

    app = configure_app()

    user_repo = InMemoryUserRepository()
    credential_repo = InMemoryUserCredentialRepository()

    async def _override_user_repo() -> InMemoryUserRepository:
        return user_repo

    async def _override_credential_repo() -> InMemoryUserCredentialRepository:
        return credential_repo

    app.dependency_overrides[api_v1._get_user_repo] = _override_user_repo
    app.dependency_overrides[api_v1._get_credential_repo] = _override_credential_repo

    async def _seed_user() -> User:
        return await user_repo.create(display_name="Alice")

    user = anyio.run(_seed_user)

    installed_auth = getattr(app.state, "auth", None)
    assert installed_auth is not None
    token_service = installed_auth.token_service
    token = token_service.issue(user_id=user.id, now=datetime.now(timezone.utc) - timedelta(seconds=10))

    with TestClient(app) as client:
        refresh_response = client.post("/v1/auth/refresh", headers={"Authorization": f"Bearer {token}"})
        assert refresh_response.status_code == 200, refresh_response.json()
        refresh_payload = refresh_response.json()
        assert refresh_payload["token_type"] == "bearer"
        assert refresh_payload["access_token"]
        assert refresh_payload["access_token"] != token

        me_response = client.get(
            "/v1/me",
            headers={"Authorization": f"Bearer {refresh_payload['access_token']}"},
        )
        assert me_response.status_code == 200
        me_payload = me_response.json()
        assert me_payload["id"] == str(user.id)
        assert me_payload["display_name"] == "Alice"


def test_refresh_rejects_expired_token(monkeypatch) -> None:
    monkeypatch.setenv("ARKLOOP_AUTH_JWT_SECRET", "test-secret-should-be-long-enough-32chars")

    app = configure_app()

    user_repo = InMemoryUserRepository()
    credential_repo = InMemoryUserCredentialRepository()

    async def _override_user_repo() -> InMemoryUserRepository:
        return user_repo

    async def _override_credential_repo() -> InMemoryUserCredentialRepository:
        return credential_repo

    app.dependency_overrides[api_v1._get_user_repo] = _override_user_repo
    app.dependency_overrides[api_v1._get_credential_repo] = _override_credential_repo

    async def _seed_user() -> User:
        return await user_repo.create(display_name="Alice")

    user = anyio.run(_seed_user)

    installed_auth = getattr(app.state, "auth", None)
    assert installed_auth is not None
    token_service = installed_auth.token_service
    expired_token = token_service.issue(
        user_id=user.id,
        now=datetime.now(timezone.utc) - timedelta(days=1),
    )

    with TestClient(app) as client:
        response = client.post(
            "/v1/auth/refresh",
            headers={"Authorization": f"Bearer {expired_token}"},
        )
    _assert_auth_error_has_trace_id(response, expected_code="auth.invalid_token")


def test_refresh_rejects_invalid_token(monkeypatch) -> None:
    monkeypatch.setenv("ARKLOOP_AUTH_JWT_SECRET", "test-secret-should-be-long-enough-32chars")

    app = configure_app()

    user_repo = InMemoryUserRepository()
    credential_repo = InMemoryUserCredentialRepository()

    async def _override_user_repo() -> InMemoryUserRepository:
        return user_repo

    async def _override_credential_repo() -> InMemoryUserCredentialRepository:
        return credential_repo

    app.dependency_overrides[api_v1._get_user_repo] = _override_user_repo
    app.dependency_overrides[api_v1._get_credential_repo] = _override_credential_repo

    with TestClient(app) as client:
        response = client.post(
            "/v1/auth/refresh",
            headers={"Authorization": "Bearer not-a-jwt"},
        )
    _assert_auth_error_has_trace_id(response, expected_code="auth.invalid_token")
