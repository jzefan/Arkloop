package app

import "testing"

func TestShouldProxyToAPI(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{path: "/v1", want: true},
		{path: "/v1/auth/login", want: true},
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
