package ipamutils

import (
	"net"
	"testing"

	_ "github.com/docker/libnetwork/testutils"
)

func TestGranularPredefined(t *testing.T) {
	PredefinedBroadNetworks = initBroadPredefinedNetworks()
	PredefinedGranularNetworks = initGranularPredefinedNetworks()

	for _, nw := range PredefinedGranularNetworks {
		if ones, bits := nw.Mask.Size(); bits != 32 || ones != 24 {
			t.Fatalf("Unexpected size for network in granular list: %v", nw)
		}
	}

	for _, nw := range PredefinedBroadNetworks {
		if ones, bits := nw.Mask.Size(); bits != 32 || (ones != 20 && ones != 16) {
			t.Fatalf("Unexpected size for network in broad list: %v", nw)
		}
	}

}

func TestInitPools(t *testing.T) {
	_, b, _ := net.ParseCIDR("10.10.10.0/24")
	list := initPools(28, b)
	if len(list) != 16 {
		t.Fatalf("Unexpected list of pools: %d", len(list))
	}
	if list[0].String() != "10.10.10.0/28" ||
		list[5].String() != "10.10.10.80/28" ||
		list[15].String() != "10.10.10.240/28" {
		t.Fatalf("Unexpected list generated: %+v", list)
	}

	_, b, _ = net.ParseCIDR("10.10.0.0/24")
	list = initPools(26, b)
	if len(list) != 4 {
		t.Fatalf("Unexpected list of pools: %d", len(list))
	}
	if list[0].String() != "10.10.0.0/26" ||
		list[1].String() != "10.10.0.64/26" ||
		list[2].String() != "10.10.0.128/26" ||
		list[3].String() != "10.10.0.192/26" {
		t.Fatalf("Unexpected list generated: %+v", list)
	}
}

func TestInitCustomPools(t *testing.T) {
	list := []*PredefinedPools{
		{
			Scope: "local",
			Base:  "172.30.30.0/24",
			Size:  25,
		},
		{
			Base: "172.30.50.0/24",
			Size: 24,
		},
		{
			Scope: "global",
			Base:  "10.10.10.0/24",
			Size:  25,
		},
	}
	expectedLocal := []string{"172.30.30.0/25", "172.30.30.128/25", "172.30.50.0/24"}
	expectedGlobal := []string{"10.10.10.0/25", "10.10.10.128/25"}

	err := InitAddressPools(list)
	if err != nil {
		t.Fatal(err)
	}

	for i, p := range PredefinedBroadNetworks {
		if p.String() != expectedLocal[i] {
			t.Fatalf("Unexpected local scope pool: %s. Expected: %s", p, expectedLocal[i])
		}
	}

	for i, p := range PredefinedGranularNetworks {
		if p.String() != expectedGlobal[i] {
			t.Fatalf("Unexpected global scope pool: %s. Expected: %s", p, expectedGlobal[i])
		}
	}

	err = InitAddressPools(list)
	if err == nil {
		t.Fatalf("Expected error, but succeeded")
	}
	if err != ErrPoolsAlreadyInitialized {
		t.Fatalf("Unexpected error message: %v", err)
	}
}
