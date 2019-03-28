package bridge

import (
	"net"
	"testing"

	"github.com/docker/libnetwork/ip6tables"
	"github.com/docker/libnetwork/portmapper"
	"github.com/docker/libnetwork/testutils"
	"github.com/vishvananda/netlink"
)

const (
	ip6tablesTestBridgeIP = "2001:db8:abcd:12::2"
)

func TestProgramIP6Table(t *testing.T) {
	t.Skip("Need to start CI docker with --ipv6")
	// Create a test bridge with a basic bridge configuration (name + IPv4).
	defer testutils.SetupTestOSContext(t)()

	nh, err := netlink.NewHandle()
	if err != nil {
		t.Fatal(err)
	}

	createTestIPv6Bridge(getBasicIPv6TestConfig(), &bridgeInterface{nlh: nh}, t)

	// Store various ip6tables chain rules we care for.
	rules := []struct {
		rule  ip6tRule
		descr string
	}{
		{ip6tRule{table: ip6tables.Filter, chain: "FORWARD", args: []string{"-d", "::1", "-i", "lo", "-o", "lo", "-j", "DROP"}}, "Test Loopback"},
		{ip6tRule{table: ip6tables.Nat, chain: "POSTROUTING", preArgs: []string{"-t", "nat"}, args: []string{"-s", ip6tablesTestBridgeIP, "!", "-o", DefaultBridgeName, "-j", "MASQUERADE"}}, "NAT Test"},
		{ip6tRule{table: ip6tables.Filter, chain: "FORWARD", args: []string{"-o", DefaultBridgeName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}}, "Test ACCEPT INCOMING"},
		{ip6tRule{table: ip6tables.Filter, chain: "FORWARD", args: []string{"-i", DefaultBridgeName, "!", "-o", DefaultBridgeName, "-j", "ACCEPT"}}, "Test ACCEPT NON_ICC OUTGOING"},
		{ip6tRule{table: ip6tables.Filter, chain: "FORWARD", args: []string{"-i", DefaultBridgeName, "-o", DefaultBridgeName, "-j", "ACCEPT"}}, "Test enable ICC"},
		{ip6tRule{table: ip6tables.Filter, chain: "FORWARD", args: []string{"-i", DefaultBridgeName, "-o", DefaultBridgeName, "-j", "DROP"}}, "Test disable ICC"},
	}

	// Assert the chain rules' insertion and removal.
	for _, c := range rules {
		assertIP6TableChainProgramming(c.rule, c.descr, t)
	}
}

func TestSetupIP6Chains(t *testing.T) {
	t.Skip("Need to start CI docker with --ipv6")
	// Create a test bridge with a basic bridge configuration (name + IPv4).
	defer testutils.SetupTestOSContext(t)()

	nh, err := netlink.NewHandle()
	if err != nil {
		t.Fatal(err)
	}

	driverconfig := &configuration{
		EnableIP6Tables: true,
	}
	d := &driver{
		config: driverconfig,
	}
	assertIPv6ChainConfig(d, t)

	config := getBasicIPv6TestConfig()
	br := &bridgeInterface{nlh: nh}
	createTestIPv6Bridge(config, br, t)

	assertIPv6BridgeConfig(config, br, d, t)

	config.EnableIPMasquerade = true
	assertBridgeConfig(config, br, d, t)

	config.EnableICC = true
	assertBridgeConfig(config, br, d, t)

	config.EnableIPMasquerade = false
	assertBridgeConfig(config, br, d, t)
}

func getBasicIPv6TestConfig() *networkConfiguration {
	config := &networkConfiguration{
		BridgeName:  DefaultBridgeName,
		AddressIPv6: &net.IPNet{IP: net.ParseIP(ip6tablesTestBridgeIP), Mask: net.CIDRMask(16, 32)}}
	return config
}

func createTestIPv6Bridge(config *networkConfiguration, br *bridgeInterface, t *testing.T) {
	if err := setupDevice(config, br); err != nil {
		t.Fatalf("Failed to create the testing Bridge: %s", err.Error())
	}
	if err := setupBridgeIPv6(config, br); err != nil {
		t.Fatalf("Failed to bring up the testing Bridge: %s", err.Error())
	}
}

// Assert base function which pushes ip6tables chain rules on insertion and removal.
func assertIP6TableChainProgramming(rule ip6tRule, descr string, t *testing.T) {
	// Add
	if err := programIP6ChainRule(rule, descr, true); err != nil {
		t.Fatalf("Failed to program ip6table rule %s: %s", descr, err.Error())
	}
	if ip6tables.Exists(rule.table, rule.chain, rule.args...) == false {
		t.Fatalf("Failed to effectively program ip6table rule: %s", descr)
	}

	// Remove
	if err := programIP6ChainRule(rule, descr, false); err != nil {
		t.Fatalf("Failed to remove ip6table rule %s: %s", descr, err.Error())
	}
	if ip6tables.Exists(rule.table, rule.chain, rule.args...) == true {
		t.Fatalf("Failed to effectively remove ip6table rule: %s", descr)
	}
}

// Assert function which create chains.
func assertIPv6ChainConfig(d *driver, t *testing.T) {
	var err error

	d.ip6tNatChain, d.ip6tFilterChain, d.ip6tIsolationChain1, d.ip6tIsolationChain2, err = setupIP6Chains(d.config)
	if err != nil {
		t.Fatal(err)
	}
}

// Assert function which pushes chains based on bridge config parameters.
func assertIPv6BridgeConfig(config *networkConfiguration, br *bridgeInterface, d *driver, t *testing.T) {
	nw := bridgeNetwork{portMapper: portmapper.New(""),
		config: config}
	nw.driver = d

	// Attempt programming of ip tables.
	err := nw.setupIP6Tables(config, br)
	if err != nil {
		t.Fatalf("%v", err)
	}
}
