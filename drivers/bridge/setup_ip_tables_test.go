package bridge

import (
	"github.com/docker/docker/pkg/iptables"
	"net"
	"testing"
	"fmt"
)

func TestProgramIPTable(t *testing.T) {
	args := []string{"FORWARD", "-o", DefaultBridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	descr := "FORWARD incoming Test"

	// Add
	if err := programIPTableEntry(args, descr, true); err != nil {
		t.Fatalf("Failed to program iptable rule: %s", err.Error())
	}

	if iptables.Exists(args...) == false {
		t.Fatalf("Failed to effectively program iptable rule: %s", descr)
	}

	// Remove
	if err := programIPTableEntry(args, "FORWARD incoming Test", false); err != nil {
		t.Fatalf("Failed to remove iptable rule: %s", err.Error())
	}

	if iptables.Exists(args...) == true {
		t.Fatalf("Failed to effectively remove iptable rule: %s", descr)
	}
}

func TestSetupIPTables(t *testing.T) {
	// Create base test bridge configuration and test different iptables settings
	br := &Interface{
		Config: &Configuration{
			BridgeName:  DefaultBridgeName,
			AddressIPv4: &net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(16, 32)},
		},
	}

	br.Config.EnableIPTables = true
	assertBridgeConfig(br, t)

	br.Config.EnableIPMasquerade = true
	assertBridgeConfig(br, t)

	br.Config.EnableICC = true
	assertBridgeConfig(br, t)

	br.Config.EnableIPMasquerade = false
	assertBridgeConfig(br, t)
}

func assertBridgeConfig(br *Interface, t *testing.T) {
	// Attempt programming of ip tables
	err := SetupIPTables(br)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Clean up, flush chain rules
	if _, err = iptables.Raw([]string{"-F", DOCKER_CHAIN}...); err != nil {
		t.Fatalf("Failed to flush %s rules: %s", DOCKER_CHAIN, err.Error())
	}

	// Clean up nat/icc/out/in rules
	addrv4, _, err := br.Addresses()
	fmt.Printf("\nGot address: %v", addrv4)
	if err = setupIPTables(br.Config.BridgeName, addrv4, br.Config.EnableICC, br.Config.EnableIPMasquerade, false); err != nil {
		t.Fatalf("Failed to unset ip table settings: %s", err.Error())
	}
}
