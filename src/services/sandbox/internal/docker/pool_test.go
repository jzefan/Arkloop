package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestBuildCreatePlan_DefaultNetwork(t *testing.T) {
	plan := buildCreatePlan(Config{
		Image:          "arkloop/sandbox-agent:latest",
		NetworkName:    "",
		GuestAgentPort: 8080,
	}, "lite")

	if !plan.dialByContainerIP {
		t.Fatalf("expected dialByContainerIP=true")
	}
	if plan.attachNetworkName != dockerInternalNetworkName(defaultAgentEgressNetworkName) {
		t.Fatalf("expected attachNetworkName=%q, got %q", dockerInternalNetworkName(defaultAgentEgressNetworkName), plan.attachNetworkName)
	}
	if plan.hostCfg.NetworkMode != container.NetworkMode(dockerInternalNetworkName(defaultAgentEgressNetworkName)) {
		t.Fatalf("expected network mode %q, got %q", dockerInternalNetworkName(defaultAgentEgressNetworkName), plan.hostCfg.NetworkMode)
	}
	if len(plan.hostCfg.PortBindings) != 0 {
		t.Fatalf("expected no port bindings, got %v", plan.hostCfg.PortBindings)
	}

	if len(plan.hostCfg.CapAdd) != 0 {
		t.Fatalf("expected CapAdd empty, got %v", plan.hostCfg.CapAdd)
	}
	if len(plan.hostCfg.CapDrop) != 1 || plan.hostCfg.CapDrop[0] != "ALL" {
		t.Fatalf("expected CapDrop=[ALL], got %v", plan.hostCfg.CapDrop)
	}
}

func TestBuildCreatePlan_CustomNetwork(t *testing.T) {
	plan := buildCreatePlan(Config{
		Image:          "arkloop/sandbox-agent:latest",
		AllowEgress:    true,
		NetworkName:    defaultAgentEgressNetworkName,
		GuestAgentPort: 8080,
	}, "lite")

	if !plan.dialByContainerIP {
		t.Fatalf("expected dialByContainerIP=true")
	}
	if plan.attachNetworkName != defaultAgentEgressNetworkName {
		t.Fatalf("expected attachNetworkName=%q, got %q", defaultAgentEgressNetworkName, plan.attachNetworkName)
	}
	if plan.hostCfg.NetworkMode != container.NetworkMode(defaultAgentEgressNetworkName) {
		t.Fatalf("expected network mode %q, got %q", defaultAgentEgressNetworkName, plan.hostCfg.NetworkMode)
	}
	if len(plan.hostCfg.PortBindings) != 0 {
		t.Fatalf("expected port bindings empty, got %v", plan.hostCfg.PortBindings)
	}

	if len(plan.hostCfg.CapAdd) != 0 {
		t.Fatalf("expected CapAdd empty, got %v", plan.hostCfg.CapAdd)
	}
	if len(plan.hostCfg.CapDrop) != 1 || plan.hostCfg.CapDrop[0] != "ALL" {
		t.Fatalf("expected CapDrop=[ALL], got %v", plan.hostCfg.CapDrop)
	}
}

func TestBuildCreatePlan_DefaultNetworkWithEgress(t *testing.T) {
	plan := buildCreatePlan(Config{
		Image:          "arkloop/sandbox-agent:latest",
		AllowEgress:    true,
		GuestAgentPort: 8080,
	}, "lite")

	if plan.attachNetworkName != defaultAgentEgressNetworkName {
		t.Fatalf("expected attachNetworkName=%q, got %q", defaultAgentEgressNetworkName, plan.attachNetworkName)
	}
	if plan.hostCfg.NetworkMode != container.NetworkMode(defaultAgentEgressNetworkName) {
		t.Fatalf("expected network mode %q, got %q", defaultAgentEgressNetworkName, plan.hostCfg.NetworkMode)
	}
	if len(plan.hostCfg.PortBindings) != 0 {
		t.Fatalf("expected no host port binding, got %v", plan.hostCfg.PortBindings)
	}
}
