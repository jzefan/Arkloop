package config

import "testing"

func TestRegistryRegisterAcceptsNumberType(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(Entry{
		Key:     "k",
		Type:    TypeNumber,
		Default: "",
		Scope:   ScopePlatform,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
}

func TestRegistryRegisterRejectsUnknownType(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(Entry{
		Key:     "k",
		Type:    "unknown",
		Default: "",
		Scope:   ScopePlatform,
	}); err == nil {
		t.Fatalf("expected error")
	}
}

