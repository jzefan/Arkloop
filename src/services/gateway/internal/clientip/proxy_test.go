package clientip

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustParseCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return n
}

func TestTrustedProxyRealIP(t *testing.T) {
	trusted := []*net.IPNet{
		mustParseCIDR(t, "10.0.0.0/8"),
		mustParseCIDR(t, "172.16.0.0/12"),
	}

	cases := []struct {
		name       string
		cidrs      []*net.IPNet
		remoteAddr string
		xff        string
		want       string
	}{
		{
			name:       "no XFF, trusted remote -> remoteAddr",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "",
			want:       "10.0.0.1",
		},
		{
			name:       "no XFF, untrusted remote -> remoteAddr",
			cidrs:      trusted,
			remoteAddr: "203.0.113.50:4321",
			xff:        "1.2.3.4",
			want:       "203.0.113.50",
		},
		{
			name:       "single client IP in XFF",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "203.0.113.10",
			want:       "203.0.113.10",
		},
		{
			name:       "spoofed leftmost IP ignored, rightmost non-trusted returned",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "1.1.1.1, 203.0.113.99, 10.0.0.2",
			want:       "203.0.113.99",
		},
		{
			name:       "multi-proxy chain, skip all trusted",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "8.8.8.8, 172.16.0.5, 10.0.0.3",
			want:       "8.8.8.8",
		},
		{
			name:       "all XFF IPs trusted -> fallback to remoteAddr",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "10.0.0.2, 172.16.0.1, 10.0.0.3",
			want:       "10.0.0.1",
		},
		{
			name:       "empty trusted CIDRs -> always remoteAddr",
			cidrs:      nil,
			remoteAddr: "1.2.3.4:5678",
			xff:        "5.6.7.8",
			want:       "1.2.3.4",
		},
		{
			name:       "invalid IPs in XFF skipped",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "not-an-ip, also-bad, 203.0.113.50",
			want:       "203.0.113.50",
		},
		{
			name:       "all XFF invalid -> fallback to remoteAddr",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "garbage, nope",
			want:       "10.0.0.1",
		},
		{
			name:       "IPv6 client through trusted proxy",
			cidrs:      []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")},
			remoteAddr: "10.0.0.1:1234",
			xff:        "2001:db8::1",
			want:       "2001:db8::1",
		},
		{
			name:       "whitespace around IPs in XFF",
			cidrs:      trusted,
			remoteAddr: "10.0.0.1:1234",
			xff:        "  203.0.113.10 , 10.0.0.2 ",
			want:       "203.0.113.10",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := &TrustedProxy{TrustedCIDRs: tc.cidrs}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			got := proxy.RealIP(req)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseCIDRList(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  int
	}{
		{"valid CIDRs", []string{"10.0.0.0/8", "172.16.0.0/12"}, 2},
		{"empty strings ignored", []string{"", "  ", "10.0.0.0/8"}, 1},
		{"malformed ignored", []string{"not-cidr", "10.0.0.0/8"}, 1},
		{"nil input", nil, 0},
		{"all invalid", []string{"bad", "worse"}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseCIDRList(tc.input)
			if len(got) != tc.want {
				t.Errorf("ParseCIDRList returned %d entries, want %d", len(got), tc.want)
			}
		})
	}
}

func TestTrustedProxyMiddlewareIntegration(t *testing.T) {
	trusted := []*net.IPNet{mustParseCIDR(t, "10.0.0.0/8")}
	proxy := &TrustedProxy{TrustedCIDRs: trusted}

	var captured string
	handler := Middleware(proxy, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9000"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.2")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if captured != "203.0.113.50" {
		t.Errorf("middleware context IP = %q, want %q", captured, "203.0.113.50")
	}
}
