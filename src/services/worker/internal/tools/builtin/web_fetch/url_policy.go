package webfetch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"

	sharedoutbound "arkloop/services/shared/outboundurl"
)

type UrlPolicyDeniedError struct {
	Reason  string
	Details map[string]any
}

func (e UrlPolicyDeniedError) Error() string {
	return fmt.Sprintf("url denied: %s", e.Reason)
}

func EnsureURLAllowed(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return UrlPolicyDeniedError{Reason: "invalid_url"}
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return UrlPolicyDeniedError{Reason: "unsupported_scheme", Details: map[string]any{"scheme": scheme}}
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return UrlPolicyDeniedError{Reason: "missing_hostname"}
	}

	lowered := strings.ToLower(strings.Trim(host, "."))
	if lowered == "localhost" || strings.HasSuffix(lowered, ".localhost") {
		return UrlPolicyDeniedError{Reason: "localhost_denied", Details: map[string]any{"hostname": host}}
	}

	ip := tryParseIP(host)
	if !ip.IsValid() {
		return nil
	}
	return EnsureIPAllowed(ip)
}

func tryParseIP(hostname string) netip.Addr {
	return sharedoutbound.ParseIP(hostname)
}

// EnsureIPAllowed 校验解析后的 IP，防止 DNS rebinding 绕过字符串级 URL 检查。
func EnsureIPAllowed(ip netip.Addr) error {
	if err := sharedoutbound.DefaultPolicy().EnsureIPAllowed(ip); err != nil {
		var denied sharedoutbound.DeniedError
		if errors.As(err, &denied) {
			return UrlPolicyDeniedError{Reason: denied.Reason, Details: denied.Details}
		}
		return err
	}
	return nil
}

// SafeDialContext 在 DNS 解析后校验全部 IP，消除 TOCTOU 窗口。
func SafeDialContext(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return sharedoutbound.DefaultPolicy().SafeDialContext(dialer)
}
