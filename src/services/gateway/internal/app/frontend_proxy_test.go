package app

import "testing"

func TestShouldProxyToAPI(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{path: "/v1", want: true},
		{path: "/v1/auth/login", want: true},
		{path: "/.well-known/openid-configuration", want: true},
		{path: "/.well-known/jwks.json", want: true},
		{path: "/", want: false},
		{path: "/settings/system", want: false},
		{path: "/assets/app.js", want: false},
	}

	for _, tc := range cases {
		if got := shouldProxyToAPI(tc.path); got != tc.want {
			t.Fatalf("path %q => %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsForbiddenPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{path: "/internal/oauth/issue", want: true},
		{path: "/internal/", want: true},
		{path: "/internalish", want: false}, // must NOT match prefix without /
		{path: "/v1/auth/login", want: false},
		{path: "/.well-known/jwks.json", want: false},
	}
	for _, tc := range cases {
		if got := isForbiddenPath(tc.path); got != tc.want {
			t.Fatalf("path %q => %v, want %v", tc.path, got, tc.want)
		}
	}
}
