//go:build desktop

package outboundurl

func defaultTrustFakeIP() bool {
	return true
}

func defaultAllowLoopbackHTTP() bool {
	return true
}
