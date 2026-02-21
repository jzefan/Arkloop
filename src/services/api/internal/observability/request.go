package observability

import "context"

type clientIPKey struct{}
type userAgentKey struct{}

func WithClientIP(ctx context.Context, ip string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if ip == "" {
		return ctx
	}
	return context.WithValue(ctx, clientIPKey{}, ip)
}

func ClientIPFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(clientIPKey{}).(string)
	return v
}

func WithUserAgent(ctx context.Context, ua string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if ua == "" {
		return ctx
	}
	return context.WithValue(ctx, userAgentKey{}, ua)
}

func UserAgentFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(userAgentKey{}).(string)
	return v
}
