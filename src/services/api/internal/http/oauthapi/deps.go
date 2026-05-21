// Package oauthapi implements the OpenID Connect Identity Provider HTTP
// surface for ArkLoop. It hosts /v1/auth/oauth/*, /.well-known/* and
// /internal/oauth/issue. See docs/oidc-design.md for the protocol details
// and §6 in particular for endpoint contracts.
package oauthapi

import (
	nethttp "net/http"

	"arkloop/services/api/internal/auth/oidc"
	"arkloop/services/api/internal/data"
)

// Deps bundles everything the OAuth handlers need. Wired in handler.go.
type Deps struct {
	Pool                data.DB
	ClientsRepo         *data.OAuthClientRepository
	AuthCodesRepo       *data.OAuthAuthorizationCodeRepository
	RefreshTokensRepo   *data.OAuthRefreshTokenRepository
	ConsentsRepo        *data.OAuthConsentRepository
	UsersRepo           *data.UserRepository
	SessionRefreshRepo  *data.RefreshTokenRepository // first-party session cookie → user_id
	OIDCService         *oidc.Service

	// Issuer is the IdP's public URL (e.g. https://arkloop.example.com).
	// Embedded in tokens and discovery metadata.
	Issuer string

	// FrontendConsentPath is the relative URL of the consent page served
	// by the web app, e.g. "/oauth/consent". The authorize endpoint
	// redirects here for first-time consent.
	FrontendConsentPath string

	// InternalServiceToken authenticates the worker calling
	// /internal/oauth/issue. Must be a high-entropy random string of
	// >= 32 bytes; passed in via env var ARKLOOP_INTERNAL_SERVICE_TOKEN.
	InternalServiceToken string

	// AccessTokenTTL / RefreshTokenTTL / IDTokenTTL in seconds.
	AccessTokenTTLSeconds  int
	RefreshTokenTTLSeconds int
	IDTokenTTLSeconds      int
}

// RegisterRoutes mounts all OAuth/OIDC endpoints on mux.
func RegisterRoutes(mux *nethttp.ServeMux, deps Deps) {
	if deps.AccessTokenTTLSeconds <= 0 {
		deps.AccessTokenTTLSeconds = 900 // 15m
	}
	if deps.RefreshTokenTTLSeconds <= 0 {
		deps.RefreshTokenTTLSeconds = 2592000 // 30d
	}
	if deps.IDTokenTTLSeconds <= 0 {
		deps.IDTokenTTLSeconds = 3600 // 1h
	}

	mux.HandleFunc("GET /v1/auth/oauth/authorize", authorize(deps))
	mux.HandleFunc("POST /v1/auth/oauth/token", token(deps))
	mux.HandleFunc("GET /v1/auth/oauth/userinfo", userinfo(deps))
	mux.HandleFunc("POST /v1/auth/oauth/revoke", revoke(deps))

	mux.HandleFunc("GET /v1/auth/oauth/consent", consentGet(deps))
	mux.HandleFunc("POST /v1/auth/oauth/consent", consentPost(deps))

	mux.HandleFunc("GET /.well-known/openid-configuration", discovery(deps))
	mux.HandleFunc("GET /.well-known/jwks.json", jwks(deps))

	mux.HandleFunc("POST /internal/oauth/issue", internalIssue(deps))
}
