package contract

import "testing"

func TestObjectKeys(t *testing.T) {
	if got := ProfileManifestKey("pref_1", "rev_1"); got != "profiles/pref_1/manifests/rev_1.json" {
		t.Fatalf("unexpected profile manifest key: %s", got)
	}
	if got := ProfileBlobKey("pref_1", "abc"); got != "profiles/pref_1/blobs/abc" {
		t.Fatalf("unexpected profile blob key: %s", got)
	}
	if got := WorkspaceManifestKey("ws_1", "rev_2"); got != "workspaces/ws_1/manifests/rev_2.json" {
		t.Fatalf("unexpected workspace manifest key: %s", got)
	}
	if got := WorkspaceBlobKey("ws_1", "def"); got != "workspaces/ws_1/blobs/def" {
		t.Fatalf("unexpected workspace blob key: %s", got)
	}
	if got := SessionRestoreKey("sh_1", "rev_3"); got != "sessions/sh_1/restore/rev_3.json" {
		t.Fatalf("unexpected session restore key: %s", got)
	}
}
