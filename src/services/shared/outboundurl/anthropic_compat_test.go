package outboundurl

import "testing"

func TestNormalizeAnthropicCompatibleBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "minimax anthropic root adds v1",
			in:   "https://api.minimaxi.com/anthropic",
			want: "https://api.minimaxi.com/anthropic/v1",
		},
		{
			name: "minimax anthropic root keeps query",
			in:   "https://api.minimaxi.com/anthropic?x=1",
			want: "https://api.minimaxi.com/anthropic/v1?x=1",
		},
		{
			name: "minimax anthropic v1 unchanged",
			in:   "https://api.minimaxi.com/anthropic/v1",
			want: "https://api.minimaxi.com/anthropic/v1",
		},
		{
			name: "other hosts unchanged",
			in:   "https://api.anthropic.com/v1",
			want: "https://api.anthropic.com/v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeAnthropicCompatibleBaseURL(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeAnthropicCompatibleBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
