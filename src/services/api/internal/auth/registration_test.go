package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRegistrationPassword(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "too_short", value: "abc123", wantErr: true},
		{name: "letters_only", value: "abcdefgh", wantErr: true},
		{name: "digits_only", value: "12345678", wantErr: true},
		{name: "too_long", value: strings.Repeat("a", 72) + "1", wantErr: true},
		{name: "valid_ascii", value: "abc12345", wantErr: false},
		{name: "valid_unicode", value: "密码123456", wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRegistrationPassword(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				var policyErr PasswordPolicyError
				if !errors.As(err, &policyErr) {
					t.Fatalf("expected PasswordPolicyError, got %T", err)
				}
				if err.Error() != passwordPolicyMessage {
					t.Fatalf("message = %q, want %q", err.Error(), passwordPolicyMessage)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
