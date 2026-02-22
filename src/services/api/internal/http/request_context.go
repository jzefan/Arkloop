package http

import (
	"net"
	"net/http"
	"strings"
)

// resolveClientIP 提取客户端真实 IP。
// 当 trustXFF=true（API 部署在 Gateway 后）时，从 X-Forwarded-For 取首个 IP。
// 直连时不信任 XFF，防止客户端伪造。
func resolveClientIP(r *http.Request, trustXFF bool) string {
	if trustXFF {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if ip, _, _ := strings.Cut(xff, ","); ip != "" {
				if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
					return parsed.String()
				}
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); parsed != nil {
			return parsed.String()
		}
		return ""
	}
	return host
}
