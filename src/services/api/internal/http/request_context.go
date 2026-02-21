package http

import (
	"net"
	"net/http"
	"strings"
)

// resolveClientIP 提取客户端真实 IP。
// 当前未引入 Gateway，暂按直连处理：直接用 RemoteAddr。
// Phase 4 启用 Gateway 后，在此改为从 X-Forwarded-For 提取首个可信 IP。
func resolveClientIP(r *http.Request) string {
	// Phase 4 Gateway 引入后启用：
	// if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
	//     if ip, _, _ := strings.Cut(xff, ","); ip != "" {
	//         if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed != nil {
	//             return parsed.String()
	//         }
	//     }
	// }

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); parsed != nil {
			return parsed.String()
		}
		return ""
	}
	return host
}
