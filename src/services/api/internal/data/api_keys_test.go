package data

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		validateFunc func(t *testing.T, hash string)
	}{
		{
			name:  "deterministic hash",
			input: "ak-test123",
			validateFunc: func(t *testing.T, hash string) {
				hash2 := HashAPIKey("ak-test123")
				if hash != hash2 {
					t.Errorf("hash not deterministic: first=%s, second=%s", hash, hash2)
				}
			},
		},
		{
			name:  "different inputs produce different outputs",
			input: "ak-test123",
			validateFunc: func(t *testing.T, hash string) {
				hash2 := HashAPIKey("ak-test456")
				if hash == hash2 {
					t.Errorf("different inputs produced same hash: %s", hash)
				}
			},
		},
		{
			name:  "empty string input",
			input: "",
			validateFunc: func(t *testing.T, hash string) {
				if hash == "" {
					t.Errorf("empty string input returned empty hash")
				}
			},
		},
		{
			name:  "valid lowercase hex string length 64",
			input: "ak-test123",
			validateFunc: func(t *testing.T, hash string) {
				if len(hash) != 64 {
					t.Errorf("hash length is %d, expected 64", len(hash))
				}
				for _, ch := range hash {
					if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
						t.Errorf("hash contains non-hex character: %c", ch)
					}
				}
			},
		},
		{
			name:  "known test vector ak-test123",
			input: "ak-test123",
			validateFunc: func(t *testing.T, hash string) {
				digest := sha256.Sum256([]byte("ak-test123"))
				expected := hex.EncodeToString(digest[:])
				if hash != expected {
					t.Errorf("hash mismatch: got=%s, expected=%s", hash, expected)
				}
			},
		},
		{
			name:  "known test vector empty string",
			input: "",
			validateFunc: func(t *testing.T, hash string) {
				digest := sha256.Sum256([]byte(""))
				expected := hex.EncodeToString(digest[:])
				if hash != expected {
					t.Errorf("hash mismatch for empty string: got=%s, expected=%s", hash, expected)
				}
			},
		},
		{
			name:  "known test vector with special characters",
			input: "ak-$%^&*()",
			validateFunc: func(t *testing.T, hash string) {
				digest := sha256.Sum256([]byte("ak-$%^&*()"))
				expected := hex.EncodeToString(digest[:])
				if hash != expected {
					t.Errorf("hash mismatch for special chars: got=%s, expected=%s", hash, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashAPIKey(tt.input)
			tt.validateFunc(t, hash)
		})
	}
}

func TestHashKey(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		validateFunc func(t *testing.T, hash string)
	}{
		{
			name:  "hashKey deterministic",
			input: "ak-test123",
			validateFunc: func(t *testing.T, hash string) {
				hash2 := hashKey("ak-test123")
				if hash != hash2 {
					t.Errorf("hashKey not deterministic: first=%s, second=%s", hash, hash2)
				}
			},
		},
		{
			name:  "hashKey valid output format",
			input: "ak-test123",
			validateFunc: func(t *testing.T, hash string) {
				if len(hash) != 64 {
					t.Errorf("hashKey output length is %d, expected 64", len(hash))
				}
			},
		},
		{
			name:  "hashKey known test vector",
			input: "ak-test123",
			validateFunc: func(t *testing.T, hash string) {
				digest := sha256.Sum256([]byte("ak-test123"))
				expected := hex.EncodeToString(digest[:])
				if hash != expected {
					t.Errorf("hashKey mismatch: got=%s, expected=%s", hash, expected)
				}
			},
		},
		{
			name:  "hashKey handles empty string",
			input: "",
			validateFunc: func(t *testing.T, hash string) {
				digest := sha256.Sum256([]byte(""))
				expected := hex.EncodeToString(digest[:])
				if hash != expected {
					t.Errorf("hashKey mismatch for empty string: got=%s, expected=%s", hash, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashKey(tt.input)
			tt.validateFunc(t, hash)
		})
	}
}

func TestHashAPIKeyConsistencyWithHashKey(t *testing.T) {
	testCases := []string{
		"ak-test123",
		"ak-longerkey123456789",
		"",
		"ak-",
		"single-word",
		"with spaces",
		"with-dash-separated-words",
	}

	for _, input := range testCases {
		t.Run("consistency_"+input, func(t *testing.T) {
			hashAPIKeyResult := HashAPIKey(input)
			hashKeyResult := hashKey(input)
			if hashAPIKeyResult != hashKeyResult {
				t.Errorf("HashAPIKey and hashKey mismatch for %q: HashAPIKey=%s, hashKey=%s",
					input, hashAPIKeyResult, hashKeyResult)
			}
		})
	}
}
