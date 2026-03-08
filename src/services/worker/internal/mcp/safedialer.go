package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	sharedoutbound "arkloop/services/shared/outboundurl"
)

// SSRFError 表示请求被 SSRF 防护拦截。
type SSRFError struct {
	Message string
}

func (e SSRFError) Error() string {
	return e.Message
}

// newSafeHTTPClient 返回一个在 DialContext 级别拦截内网地址的 HTTP 客户端。
// DNS 解析后检查实际 IP，防止 DNS rebinding 攻击。
func newSafeHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("mcp: ssrf: invalid addr %q: %w", addr, err)
		}

		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("mcp: ssrf: resolve %q: %w", host, err)
		}

		for _, ip := range ips {
			if isDeniedIP(ip.Unmap()) {
				return nil, SSRFError{Message: fmt.Sprintf("mcp: ssrf: denied ip %s for host %s", ip, host)}
			}
		}

		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].Unmap().String(), port))
	}

	return &http.Client{
		Timeout:   0,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("mcp: ssrf: too many redirects")
			}
			if err := validateURL(req.URL); err != nil {
				return err
			}
			return nil
		},
	}
}

// validateURL 在发起请求前做 URL 级别的预检查。
func validateURL(u *url.URL) error {
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return SSRFError{Message: fmt.Sprintf("mcp: ssrf: unsupported scheme %q", scheme)}
	}

	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return SSRFError{Message: "mcp: ssrf: empty hostname"}
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return SSRFError{Message: fmt.Sprintf("mcp: ssrf: denied hostname %q", host)}
	}

	if ip := sharedoutbound.ParseIP(host); ip.IsValid() {
		if isDeniedIP(ip) {
			return SSRFError{Message: fmt.Sprintf("mcp: ssrf: denied ip %s", ip)}
		}
	}

	return nil
}

func isDeniedIP(ip netip.Addr) bool {
	if isCloudMetadata(ip) {
		return true
	}
	return sharedoutbound.DefaultPolicy().EnsureIPAllowed(ip.Unmap()) != nil
}

// isCloudMetadata 检查 AWS/GCP/Azure 元数据服务地址。
func isCloudMetadata(ip netip.Addr) bool {
	metadata := []netip.Addr{
		netip.MustParseAddr("169.254.169.254"),
		netip.MustParseAddr("fd00:ec2::254"),
	}
	for _, m := range metadata {
		if ip == m {
			return true
		}
	}
	return false
}
