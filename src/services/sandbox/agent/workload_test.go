package main

import "testing"

func TestBaseWorkloadEnvDisablesPythonBytecode(t *testing.T) {
	env := baseWorkloadEnv()
	if env["PYTHONDONTWRITEBYTECODE"] != "1" {
		t.Fatalf("expected PYTHONDONTWRITEBYTECODE=1, got %q", env["PYTHONDONTWRITEBYTECODE"])
	}
}
