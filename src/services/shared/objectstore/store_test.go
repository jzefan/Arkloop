package objectstore

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestExpirationLifecycleConfiguration(t *testing.T) {
	config := expirationLifecycleConfiguration(7)
	if config == nil {
		t.Fatal("expected lifecycle configuration")
	}
	if len(config.Rules) != 1 {
		t.Fatalf("expected one rule, got %d", len(config.Rules))
	}
	rule := config.Rules[0]
	if rule.Status != types.ExpirationStatusEnabled {
		t.Fatalf("unexpected rule status: %s", rule.Status)
	}
	if rule.Expiration == nil || rule.Expiration.Days == nil || *rule.Expiration.Days != 7 {
		t.Fatalf("unexpected expiration days: %#v", rule.Expiration)
	}
	if rule.Filter == nil || rule.Filter.Prefix == nil {
		t.Fatalf("unexpected lifecycle filter: %#v", rule.Filter)
	}
	if *rule.Filter.Prefix != "" {
		t.Fatalf("unexpected filter prefix: %q", *rule.Filter.Prefix)
	}
}
