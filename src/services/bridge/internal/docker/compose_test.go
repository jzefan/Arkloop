package docker

import "testing"

func TestParsePSEntries(t *testing.T) {
	raw := []byte(`{"Service":"gateway","State":"running","Health":"healthy"}
{"Service":"openviking","State":"exited","Health":""}
`)

	entries := parsePSEntries(raw)
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	if entries[0].Service != "gateway" || entries[0].State != "running" {
		t.Fatalf("entries[0] = %#v, want gateway/running", entries[0])
	}
	if entries[1].Service != "openviking" || entries[1].State != "exited" {
		t.Fatalf("entries[1] = %#v, want openviking/exited", entries[1])
	}
}

func TestParsePSEntriesSkipsInvalidLines(t *testing.T) {
	raw := []byte(`{"Service":"gateway","State":"running","Health":"healthy"}
not-json
{"Service":"bridge","State":"running","Health":""}
`)

	entries := parsePSEntries(raw)
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	if entries[0].Service != "gateway" {
		t.Fatalf("entries[0].Service = %q, want gateway", entries[0].Service)
	}
	if entries[1].Service != "bridge" {
		t.Fatalf("entries[1].Service = %q, want bridge", entries[1].Service)
	}
}
