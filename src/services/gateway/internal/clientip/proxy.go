package clientip

import (
	"net"
	"net/http"
	"strings"
)

// TrustedProxy 适用于任意 CDN 或内部负载均衡器前置的场景。
// 仅当 RemoteAddr 属于 TrustedCIDRs 时，取 X-Forwarded-For 链最左端的 IP（即原始客户端 IP）。
// 否则降级为 RemoteAddr。
type TrustedProxy struct {
	TrustedCIDRs []*net.IPNet
}

func (t *TrustedProxy) RealIP(r *http.Request) string {
	remoteIP := remoteAddrIP(r)

	if len(t.TrustedCIDRs) == 0 || !inCIDRList(remoteIP, t.TrustedCIDRs) {
		return remoteIP
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remoteIP
	}

	// XFF 格式："client, proxy1, proxy2"，最左端为原始客户端 IP。
	first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	if parsed := net.ParseIP(first); parsed != nil {
		return parsed.String()
	}

	return remoteIP
}

// ParseCIDRList 解析 CIDR 字符串列表，忽略空行和格式错误的条目。
func ParseCIDRList(cidrs []string) []*net.IPNet {
	var result []*net.IPNet
	for _, s := range cidrs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		_, network, err := net.ParseCIDR(s)
		if err != nil {
			continue
		}
		result = append(result, network)
	}
	return result
}
