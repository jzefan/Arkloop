package clientip

import (
	"net"
	"net/http"
)

// Cloudflare 适用于 Cloudflare 前置的场景。
// 仅当 RemoteAddr 属于配置的 Cloudflare IP 段时，才读取 CF-Connecting-IP header。
// 否则降级为 RemoteAddr，防止客户端伪造 CF 头绕过 IP 识别。
type Cloudflare struct {
	// TrustedCIDRs 是 Cloudflare 的 IP 段列表（IPv4 + IPv6）。
	// 若为空则不做 CDN IP 来源验证，直接读 CF-Connecting-IP（不推荐，仅限内网环境）。
	TrustedCIDRs []*net.IPNet
}

func (c *Cloudflare) RealIP(r *http.Request) string {
	remoteIP := remoteAddrIP(r)

	if len(c.TrustedCIDRs) > 0 && !inCIDRList(remoteIP, c.TrustedCIDRs) {
		return remoteIP
	}

	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		if parsed := net.ParseIP(cfIP); parsed != nil {
			return parsed.String()
		}
	}

	return remoteIP
}

func inCIDRList(ipStr string, cidrs []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
