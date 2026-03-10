package skillstore

import (
	"archive/tar"
	"bytes"
	"testing"

	"arkloop/services/shared/workspaceblob"
)

func TestDecodeManifestAndBundle(t *testing.T) {
	bundle := buildTestBundle(t, map[string]string{
		"skill.yaml": "skill_key: grep-helper\nversion: \"1\"\ndisplay_name: Grep Helper\ninstruction_path: SKILL.md\n",
		"SKILL.md":   "Use grep carefully.\n",
		"scripts/run.sh": "#!/bin/sh\necho ok\n",
	})
	manifest, err := DecodeManifest([]byte(`{"skill_key":"grep-helper","version":"1","display_name":"Grep Helper","instruction_path":"SKILL.md","manifest_key":"skills/grep-helper/1/manifest.json","bundle_key":"skills/grep-helper/1/bundle.tar.zst","files_prefix":"skills/grep-helper/1/files/"}`))
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	image, err := DecodeBundle(bundle)
	if err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if err := ValidateBundleAgainstManifest(manifest, image); err != nil {
		t.Fatalf("validate bundle: %v", err)
	}
}

func TestDecodeBundleRejectsPathTraversal(t *testing.T) {
	bundle := buildTestBundle(t, map[string]string{
		"skill.yaml": "skill_key: grep-helper\nversion: \"1\"\ndisplay_name: Grep Helper\ninstruction_path: SKILL.md\n",
		"SKILL.md":   "doc\n",
		"../escape":  "boom",
	})
	if _, err := DecodeBundle(bundle); err == nil {
		t.Fatal("expected path traversal bundle to fail")
	}
}

func buildTestBundle(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var tarBuffer bytes.Buffer
	writer := tar.NewWriter(&tarBuffer)
	for name, content := range files {
		data := []byte(content)
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write tar data: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	encoded, err := workspaceblob.Encode(tarBuffer.Bytes())
	if err != nil {
		t.Fatalf("encode zstd: %v", err)
	}
	return encoded
}
