package firecracker

import "testing"

func TestSubnetAt(t *testing.T) {
	manager, err := NewNetworkManager(NetworkConfig{
		TapPrefix:       "arktap",
		AddressPoolCIDR: "172.29.0.0/16",
	})
	if err != nil {
		t.Fatalf("new network manager failed: %v", err)
	}

	subnet, host, guest, err := subnetAt(manager.pool, 3)
	if err != nil {
		t.Fatalf("subnetAt failed: %v", err)
	}
	if got, want := subnet.String(), "172.29.0.12/30"; got != want {
		t.Fatalf("unexpected subnet: got %s want %s", got, want)
	}
	if got, want := host.String(), "172.29.0.13"; got != want {
		t.Fatalf("unexpected host ip: got %s want %s", got, want)
	}
	if got, want := guest.String(), "172.29.0.14"; got != want {
		t.Fatalf("unexpected guest ip: got %s want %s", got, want)
	}
}

