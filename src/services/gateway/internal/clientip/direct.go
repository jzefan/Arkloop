package clientip

import (
	"net"
	"net/http"
	"strings"
)

// Direct 只信任 TCP 连接的 RemoteAddr，适用于 Gateway 直接暴露在互联网的场景。
type Direct struct{}

func (Direct) RealIP(r *http.Request) string {
	return remoteAddrIP(r)
}

func remoteAddrIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); parsed != nil {
			return parsed.String()
		}
		return ""
	}
	return host
}
