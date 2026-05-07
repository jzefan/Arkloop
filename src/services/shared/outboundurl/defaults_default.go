//go:build !desktop

package outboundurl

func defaultTrustFakeIP() bool {
	return false
}

func defaultAllowLoopbackHTTP() bool {
	return false
}
