package clientip

import (
	"context"
	"net/http"
)

type contextKey struct{}

// Resolver 从请求中提取真实客户端 IP。
type Resolver interface {
	RealIP(r *http.Request) string
}

// Middleware 将 Resolver 解析结果写入 context，供后续中间件读取。
func Middleware(resolver Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := resolver.RealIP(r)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, ip)))
	})
}

// FromContext 从 context 中取出已解析的客户端 IP。
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(contextKey{}).(string)
	return v
}
