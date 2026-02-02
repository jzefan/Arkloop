from __future__ import annotations

from packages.llm_routing import (
    ProviderCredential,
    ProviderRouteDenied,
    ProviderRouter,
    ProviderRouteRule,
    ProviderRoutingConfig,
    SelectedProviderRoute,
)


def test_provider_router_selects_default_route_when_no_route_id() -> None:
    config = ProviderRoutingConfig(
        default_route_id="default",
        credentials=(
            ProviderCredential(id="stub_default", scope="platform", provider_kind="stub"),
        ),
        routes=(
            ProviderRouteRule(id="default", model="stub", credential_id="stub_default"),
        ),
    )
    router = ProviderRouter(config=config)

    decision = router.decide(input_json={}, byok_enabled=False)
    assert isinstance(decision, SelectedProviderRoute)
    assert decision.route.id == "default"
    assert decision.credential.id == "stub_default"


def test_provider_router_denies_byok_credential_when_org_byok_disabled() -> None:
    config = ProviderRoutingConfig(
        default_route_id="default",
        credentials=(
            ProviderCredential(id="stub_default", scope="platform", provider_kind="stub"),
            ProviderCredential(
                id="org_openai",
                scope="org",
                provider_kind="openai",
                api_key_env="ORG_OPENAI_API_KEY",
                base_url="https://example.test/v1",
                openai_api_mode="chat_completions",
            ),
        ),
        routes=(
            ProviderRouteRule(id="default", model="stub", credential_id="stub_default"),
            ProviderRouteRule(id="byok", model="gpt-test", credential_id="org_openai"),
        ),
    )
    router = ProviderRouter(config=config)

    decision = router.decide(input_json={"route_id": "byok"}, byok_enabled=False)
    assert isinstance(decision, ProviderRouteDenied)
    assert decision.error_class == "policy.denied"
    assert decision.code == "policy.byok_disabled"


def test_provider_router_allows_byok_credential_when_org_byok_enabled() -> None:
    config = ProviderRoutingConfig(
        default_route_id="default",
        credentials=(
            ProviderCredential(id="stub_default", scope="platform", provider_kind="stub"),
            ProviderCredential(
                id="org_openai",
                scope="org",
                provider_kind="openai",
                api_key_env="ORG_OPENAI_API_KEY",
                base_url="https://example.test/v1",
                openai_api_mode="chat_completions",
            ),
        ),
        routes=(
            ProviderRouteRule(id="default", model="stub", credential_id="stub_default"),
            ProviderRouteRule(id="byok", model="gpt-test", credential_id="org_openai"),
        ),
    )
    router = ProviderRouter(config=config)

    decision = router.decide(input_json={"route_id": "byok"}, byok_enabled=True)
    assert isinstance(decision, SelectedProviderRoute)
    assert decision.route.id == "byok"
    assert decision.credential.id == "org_openai"

