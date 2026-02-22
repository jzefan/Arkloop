package ipfilter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"
)

type rules struct {
	Allowlist []string `json:"allowlist"`
	Blocklist []string `json:"blocklist"`
}

// Filter 从 Redis 缓存加载 org IP 规则并执行过滤检查，同时支持从 API Key 缓存提取 org_id。
type Filter struct {
	redis *redis.Client
}

func NewFilter(redisClient *redis.Client) *Filter {
	return &Filter{redis: redisClient}
}

// Middleware 返回检查请求 IP 的 HTTP 中间件。
// 无 org_id 或 Redis 不可用时 fail-open，让下游 API 处理认证。
func (f *Filter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := extractOrgIDWithRedis(r.Header.Get("Authorization"), f.redis, r.Context())
		if orgID != "" {
			clientIP := extractClientIP(r)
			if clientIP != "" {
				if blocked := f.check(r.Context(), orgID, clientIP); blocked {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"code":"ip.blocked","message":"Forbidden"}`))
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// check 返回 true 表示请求应被拒绝。
func (f *Filter) check(ctx context.Context, orgID, clientIP string) bool {
	r, err := f.loadRules(ctx, orgID)
	if err != nil || (len(r.Allowlist) == 0 && len(r.Blocklist) == 0) {
		return false
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	// blocklist 优先：命中则拒绝
	for _, cidr := range r.Blocklist {
		if matchCIDR(ip, cidr) {
			return true
		}
	}

	// allowlist 存在时：未命中则拒绝
	if len(r.Allowlist) > 0 {
		for _, cidr := range r.Allowlist {
			if matchCIDR(ip, cidr) {
				return false
			}
		}
		return true
	}

	return false
}

func (f *Filter) loadRules(ctx context.Context, orgID string) (rules, error) {
	if f.redis == nil {
		return rules{}, fmt.Errorf("redis not configured")
	}

	key := fmt.Sprintf("arkloop:ip_rules:%s", orgID)
	raw, err := f.redis.Get(ctx, key).Bytes()
	if err != nil {
		// cache miss or Redis error → fail-open
		return rules{}, err
	}

	var r rules
	if err := json.Unmarshal(raw, &r); err != nil {
		return rules{}, err
	}
	return r, nil
}

func matchCIDR(ip net.IP, cidr string) bool {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return network.Contains(ip)
}

// extractClientIP 从 TCP 连接的 RemoteAddr 提取 IP（不信任 XFF，防止伪造）。
func extractClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		if parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); parsed != nil {
			return parsed.String()
		}
		return ""
	}
	return host
}
