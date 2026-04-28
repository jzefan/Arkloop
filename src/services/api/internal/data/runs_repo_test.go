package data

import "testing"

func TestApplyContinuationMetadata(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		meta := applyContinuationMetadata(nil)
		if meta["continuation_source"] != "none" {
			t.Fatalf("expected continuation_source none, got %#v", meta["continuation_source"])
		}
		if loop, ok := meta["continuation_loop"].(bool); !ok || loop {
			t.Fatalf("expected continuation_loop false, got %#v", meta["continuation_loop"])
		}
		if _, ok := meta["continuation_response"]; ok {
			t.Fatalf("did not expect continuation_response for nil resume, got %#v", meta["continuation_response"])
		}
	})
}
