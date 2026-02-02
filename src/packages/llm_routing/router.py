from __future__ import annotations

from dataclasses import dataclass, field
import json
import os
import re
from typing import Any, Literal, Mapping, TypeAlias

from packages.agent_core.tools import POLICY_DENIED_CODE, sha256_json
from packages.config import load_dotenv_if_enabled

ProviderKind: TypeAlias = Literal["stub", "openai", "anthropic"]
CredentialScope: TypeAlias = Literal["platform", "org"]

_ROUTING_JSON_ENV = "ARKLOOP_PROVIDER_ROUTING_JSON"
_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$")
_ENV_NAME_RE = re.compile(r"^[A-Z][A-Z0-9_]{0,63}$")


def _as_mapping(value: object, *, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ValueError(f"{label} 必须为 JSON 对象")
    return value


def _as_str(value: object, *, label: str) -> str:
    if not isinstance(value, str):
        raise ValueError(f"{label} 必须为字符串")
    cleaned = value.strip()
    if not cleaned:
        raise ValueError(f"{label} 不能为空")
    return cleaned


def _validate_id(value: str, *, label: str) -> str:
    if not _ID_RE.fullmatch(value):
        raise ValueError(f"{label} 不合法: {value}")
    return value


def _validate_env_name(value: str, *, label: str) -> str:
    if not _ENV_NAME_RE.fullmatch(value):
        raise ValueError(f"{label} 必须为合法环境变量名: {value}")
    return value


def _parse_provider_kind(value: str, *, label: str) -> ProviderKind:
    candidate = value.strip().casefold()
    if candidate in {"stub", "openai", "anthropic"}:
        return candidate  # type: ignore[return-value]
    raise ValueError(f"{label} 必须为 stub/openai/anthropic")


def _parse_credential_scope(value: str, *, label: str) -> CredentialScope:
    candidate = value.strip().casefold()
    if candidate in {"platform", "org"}:
        return candidate  # type: ignore[return-value]
    raise ValueError(f"{label} 必须为 platform/org")


def _parse_openai_api_mode(value: object) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError("openai_api_mode 必须为字符串")
    cleaned = value.strip()
    if not cleaned:
        raise ValueError("openai_api_mode 不能为空")
    if cleaned not in {"auto", "responses", "chat_completions"}:
        raise ValueError("openai_api_mode 必须为 auto/responses/chat_completions")
    return cleaned


@dataclass(frozen=True, slots=True)
class ProviderCredential:
    id: str
    scope: CredentialScope
    provider_kind: ProviderKind
    api_key_env: str | None = None
    base_url: str | None = None
    openai_api_mode: str | None = None
    advanced_json: Mapping[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        _validate_id(self.id, label="credential.id")
        if self.api_key_env is not None:
            _validate_env_name(self.api_key_env, label="credential.api_key_env")

        if self.provider_kind == "openai":
            if self.openai_api_mode is None:
                raise ValueError("OpenAI credential 必须指定 openai_api_mode")
        else:
            if self.openai_api_mode is not None:
                raise ValueError("仅 OpenAI credential 允许设置 openai_api_mode")

        if self.provider_kind != "stub" and not self.api_key_env:
            raise ValueError("非 stub credential 必须提供 api_key_env（仅保存 env 名，不保存明文）")

        if not isinstance(self.advanced_json, Mapping):
            raise ValueError("credential.advanced_json 必须为 JSON 对象")

    def to_public_json(self) -> dict[str, Any]:
        payload: dict[str, Any] = {
            "credential_id": self.id,
            "scope": self.scope,
            "provider_kind": self.provider_kind,
        }
        if self.base_url is not None:
            payload["base_url"] = self.base_url
        if self.openai_api_mode is not None:
            payload["openai_api_mode"] = self.openai_api_mode
        if self.advanced_json:
            payload["advanced_json_sha256"] = sha256_json(self.advanced_json)
        return payload


@dataclass(frozen=True, slots=True)
class ProviderRouteRule:
    id: str
    model: str
    credential_id: str
    when: Mapping[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        _validate_id(self.id, label="route.id")
        _validate_id(self.credential_id, label="route.credential_id")
        if not isinstance(self.when, Mapping):
            raise ValueError("route.when 必须为 JSON 对象")

    def matches(self, input_json: Mapping[str, Any]) -> bool:
        if not self.when:
            return True
        for key, expected in self.when.items():
            if input_json.get(key) != expected:
                return False
        return True


@dataclass(frozen=True, slots=True)
class ProviderRoutingConfig:
    default_route_id: str
    credentials: tuple[ProviderCredential, ...]
    routes: tuple[ProviderRouteRule, ...]

    @classmethod
    def default(cls) -> "ProviderRoutingConfig":
        credential = ProviderCredential(id="stub_default", scope="platform", provider_kind="stub")
        route = ProviderRouteRule(id="default", model="stub", credential_id=credential.id, when={})
        return cls(default_route_id=route.id, credentials=(credential,), routes=(route,))

    @classmethod
    def from_env(cls) -> "ProviderRoutingConfig":
        load_dotenv_if_enabled(override=False)
        raw = os.getenv(_ROUTING_JSON_ENV)
        if not raw:
            return cls.default()

        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError as exc:
            raise ValueError(f"环境变量 {_ROUTING_JSON_ENV} 不是合法 JSON") from exc

        root = _as_mapping(parsed, label=_ROUTING_JSON_ENV)
        default_route_id = _validate_id(
            _as_str(root.get("default_route_id"), label="default_route_id"),
            label="default_route_id",
        )

        credentials_raw = root.get("credentials")
        if not isinstance(credentials_raw, list) or not credentials_raw:
            raise ValueError("credentials 必须为非空数组")

        credentials: list[ProviderCredential] = []
        seen_credential_ids: set[str] = set()
        for index, item in enumerate(credentials_raw):
            obj = _as_mapping(item, label=f"credentials[{index}]")
            cred_id = _validate_id(_as_str(obj.get("id"), label="credentials[].id"), label="credentials[].id")
            if cred_id in seen_credential_ids:
                raise ValueError(f"credential.id 重复: {cred_id}")
            seen_credential_ids.add(cred_id)

            scope = _parse_credential_scope(_as_str(obj.get("scope"), label="credentials[].scope"), label="scope")
            provider_kind = _parse_provider_kind(
                _as_str(obj.get("provider_kind"), label="credentials[].provider_kind"),
                label="provider_kind",
            )
            api_key_env = obj.get("api_key_env")
            if api_key_env is not None:
                api_key_env = _validate_env_name(
                    _as_str(api_key_env, label="credentials[].api_key_env"),
                    label="credentials[].api_key_env",
                )
            base_url = obj.get("base_url")
            if base_url is not None:
                base_url = _as_str(base_url, label="credentials[].base_url")
            openai_api_mode = _parse_openai_api_mode(obj.get("openai_api_mode"))
            advanced_json = obj.get("advanced_json") or {}
            advanced_json = _as_mapping(advanced_json, label="credentials[].advanced_json")

            credentials.append(
                ProviderCredential(
                    id=cred_id,
                    scope=scope,
                    provider_kind=provider_kind,
                    api_key_env=api_key_env,
                    base_url=base_url,
                    openai_api_mode=openai_api_mode,
                    advanced_json=advanced_json,
                )
            )

        routes_raw = root.get("routes")
        if not isinstance(routes_raw, list) or not routes_raw:
            raise ValueError("routes 必须为非空数组")

        routes: list[ProviderRouteRule] = []
        seen_route_ids: set[str] = set()
        for index, item in enumerate(routes_raw):
            obj = _as_mapping(item, label=f"routes[{index}]")
            route_id = _validate_id(_as_str(obj.get("id"), label="routes[].id"), label="routes[].id")
            if route_id in seen_route_ids:
                raise ValueError(f"route.id 重复: {route_id}")
            seen_route_ids.add(route_id)
            model = _as_str(obj.get("model"), label="routes[].model")
            credential_id = _validate_id(
                _as_str(obj.get("credential_id"), label="routes[].credential_id"),
                label="routes[].credential_id",
            )
            when = obj.get("when") or {}
            when = _as_mapping(when, label="routes[].when")

            routes.append(
                ProviderRouteRule(
                    id=route_id,
                    model=model,
                    credential_id=credential_id,
                    when=when,
                )
            )

        credentials_by_id = {cred.id: cred for cred in credentials}
        for route in routes:
            if route.credential_id not in credentials_by_id:
                raise ValueError(f"route.credential_id 不存在: {route.credential_id}")

        if default_route_id not in seen_route_ids:
            raise ValueError(f"default_route_id 不存在: {default_route_id}")

        return cls(
            default_route_id=default_route_id,
            credentials=tuple(credentials),
            routes=tuple(routes),
        )

    def get_credential(self, credential_id: str) -> ProviderCredential:
        for item in self.credentials:
            if item.id == credential_id:
                return item
        raise KeyError(credential_id)

    def get_route(self, route_id: str) -> ProviderRouteRule:
        for item in self.routes:
            if item.id == route_id:
                return item
        raise KeyError(route_id)


@dataclass(frozen=True, slots=True)
class SelectedProviderRoute:
    route: ProviderRouteRule
    credential: ProviderCredential

    def to_run_event_data_json(self) -> dict[str, Any]:
        payload = {
            "route_id": self.route.id,
            "model": self.route.model,
        }
        payload.update(self.credential.to_public_json())
        return payload


@dataclass(frozen=True, slots=True)
class ProviderRouteDenied:
    error_class: str
    code: str
    message: str
    details: Mapping[str, Any] = field(default_factory=dict)

    def to_run_failed_data_json(self) -> dict[str, Any]:
        payload: dict[str, Any] = {
            "error_class": self.error_class,
            "code": self.code,
            "message": self.message,
        }
        if self.details:
            payload["details"] = dict(self.details)
        return payload


ProviderRouteDecision: TypeAlias = SelectedProviderRoute | ProviderRouteDenied


class ProviderRouter:
    def __init__(self, *, config: ProviderRoutingConfig) -> None:
        self._config = config

    def decide(
        self,
        *,
        input_json: Mapping[str, Any],
        byok_enabled: bool,
    ) -> ProviderRouteDecision:
        requested_route_id = input_json.get("route_id")
        if requested_route_id is not None and not isinstance(requested_route_id, str):
            return ProviderRouteDenied(
                error_class=POLICY_DENIED_CODE,
                code="policy.invalid_route_id",
                message="route_id 必须为字符串",
            )

        if isinstance(requested_route_id, str) and requested_route_id.strip():
            route_id = requested_route_id.strip()
            try:
                route = self._config.get_route(route_id)
            except KeyError:
                return ProviderRouteDenied(
                    error_class=POLICY_DENIED_CODE,
                    code="policy.route_not_found",
                    message="路由不存在",
                    details={"route_id": route_id},
                )
        else:
            route = self._pick_first_matching_route(input_json)

        credential = self._config.get_credential(route.credential_id)

        if credential.scope == "org" and not byok_enabled:
            return ProviderRouteDenied(
                error_class=POLICY_DENIED_CODE,
                code="policy.byok_disabled",
                message="该组织未启用 BYOK",
                details={"route_id": route.id, "credential_id": credential.id},
            )

        return SelectedProviderRoute(route=route, credential=credential)

    def _pick_first_matching_route(self, input_json: Mapping[str, Any]) -> ProviderRouteRule:
        for route in self._config.routes:
            if route.id == self._config.default_route_id:
                continue
            if route.matches(input_json):
                return route
        return self._config.get_route(self._config.default_route_id)


__all__ = [
    "CredentialScope",
    "ProviderCredential",
    "ProviderKind",
    "ProviderRouteDecision",
    "ProviderRouteDenied",
    "ProviderRouteRule",
    "ProviderRouter",
    "ProviderRoutingConfig",
    "SelectedProviderRoute",
]
