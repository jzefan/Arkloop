package http

import "testing"

func TestCopyNameSeed(t *testing.T) {
	cases := []struct {
		name           string
		input          string
		wantBaseName   string
		wantNextSuffix int
	}{
		{
			name:           "plain name",
			input:          "primary-openai",
			wantBaseName:   "primary-openai",
			wantNextSuffix: 1,
		},
		{
			name:           "name with number suffix",
			input:          "primary-openai-3",
			wantBaseName:   "primary-openai",
			wantNextSuffix: 4,
		},
		{
			name:           "name with invalid suffix",
			input:          "primary-openai-xx",
			wantBaseName:   "primary-openai-xx",
			wantNextSuffix: 1,
		},
		{
			name:           "empty name fallback",
			input:          "   ",
			wantBaseName:   "credential",
			wantNextSuffix: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseName, nextSuffix := copyNameSeed(tc.input)
			if baseName != tc.wantBaseName || nextSuffix != tc.wantNextSuffix {
				t.Fatalf("want (%q, %d), got (%q, %d)", tc.wantBaseName, tc.wantNextSuffix, baseName, nextSuffix)
			}
		})
	}
}

func TestNextCopiedCredentialName(t *testing.T) {
	name := nextCopiedCredentialName("prod-openai", map[string]struct{}{
		"prod-openai":   {},
		"prod-openai-1": {},
		"prod-openai-2": {},
	})
	if name != "prod-openai-3" {
		t.Fatalf("want prod-openai-3, got %s", name)
	}

	name = nextCopiedCredentialName("prod-openai-2", map[string]struct{}{
		"prod-openai-2": {},
		"prod-openai-3": {},
	})
	if name != "prod-openai-4" {
		t.Fatalf("want prod-openai-4, got %s", name)
	}
}
