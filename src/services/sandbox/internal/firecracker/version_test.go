package firecracker

import "testing"

func TestParseVersion(t *testing.T) {
	version, err := ParseVersion("Firecracker v1.12.1")
	if err != nil {
		t.Fatalf("parse version failed: %v", err)
	}
	if version != (Version{Major: 1, Minor: 12, Patch: 1}) {
		t.Fatalf("unexpected version: %#v", version)
	}
}

func TestVersionLess(t *testing.T) {
	if !(Version{Major: 1, Minor: 11, Patch: 9}).Less(MinSnapshotTapPatchVersion) {
		t.Fatal("expected lower version to compare less")
	}
	if (Version{Major: 1, Minor: 12, Patch: 1}).Less(MinSnapshotTapPatchVersion) {
		t.Fatal("expected equal version to compare not less")
	}
}
