package bridge

import (
	"errors"
	"fmt"
	"net"

	"github.com/docker/libnetwork/firewallapi"
	"github.com/docker/libnetwork/iptables"
	"github.com/docker/libnetwork/nftables"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// DockerChain: DOCKER iptable chain name
const (
	DockerChain = "DOCKER"
	// Isolation between bridge networks is achieved in two stages by means
	// of the following two chains in the filter table. The first chain matches
	// on the source interface being a bridge network's bridge and the
	// destination being a different interface. A positive match leads to the
	// second isolation chain. No match returns to the parent chain. The second
	// isolation chain matches on destination interface being a bridge network's
	// bridge. A positive match identifies a packet originated from one bridge
	// network's bridge destined to another bridge network's bridge and will
	// result in the packet being dropped. No match returns to the parent chain.
	IsolationChain1 = "DOCKER-ISOLATION-STAGE-1"
	IsolationChain2 = "DOCKER-ISOLATION-STAGE-2"
)

func setupIPChains(config *configuration, version firewallapi.IPVersion) (firewallapi.FirewallChain, firewallapi.FirewallChain, firewallapi.FirewallChain, firewallapi.FirewallChain, error) {
	// Sanity check.
	if config.EnableIPTables == false {
		return nil, nil, nil, nil, errors.New("cannot create new chains, EnableIPTable is disabled")
	}

	var table firewallapi.FirewallTable

	if config.EnableNFTables {
		table = nftables.GetTable(nftables.IPVersion(version))
	} else {
		table = iptables.GetTable(iptables.IPVersion(version))
	}

	hairpinMode := !config.EnableUserlandProxy

	natChain, err := table.NewChain(DockerChain, iptables.Nat, hairpinMode)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create NAT chain %s: %v", DockerChain, err)
	}
	defer func() {
		if err != nil {
			if err := table.RemoveExistingChain(DockerChain, iptables.Nat); err != nil {
				logrus.Warnf("failed on removing iptables NAT chain %s on cleanup: %v", DockerChain, err)
			}
		}
	}()

	filterChain, err := table.NewChain(DockerChain, iptables.Filter, false)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create FILTER chain %s: %v", DockerChain, err)
	}
	defer func() {
		if err != nil {
			if err := table.RemoveExistingChain(DockerChain, iptables.Filter); err != nil {
				logrus.Warnf("failed on removing iptables FILTER chain %s on cleanup: %v", DockerChain, err)
			}
		}
	}()

	isolationChain1, err := table.NewChain(IsolationChain1, iptables.Filter, false)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create FILTER isolation chain: %v", err)
	}
	defer func() {
		if err != nil {
			if err := table.RemoveExistingChain(IsolationChain1, iptables.Filter); err != nil {
				logrus.Warnf("failed on removing iptables FILTER chain %s on cleanup: %v", IsolationChain1, err)
			}
		}
	}()

	isolationChain2, err := table.NewChain(IsolationChain2, iptables.Filter, false)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create FILTER isolation chain: %v", err)
	}
	defer func() {
		if err != nil {
			if err := table.RemoveExistingChain(IsolationChain2, iptables.Filter); err != nil {
				logrus.Warnf("failed on removing iptables FILTER chain %s on cleanup: %v", IsolationChain2, err)
			}
		}
	}()

	if err := table.AddReturnRule(IsolationChain1); err != nil {
		return nil, nil, nil, nil, err
	}

	if err := table.AddReturnRule(IsolationChain2); err != nil {
		return nil, nil, nil, nil, err
	}

	return natChain, filterChain, isolationChain1, isolationChain2, nil
}

func (n *bridgeNetwork) setupIP4Tables(config *networkConfiguration, i *bridgeInterface) error {
	d := n.driver
	d.Lock()
	driverConfig := d.config
	d.Unlock()

	// Sanity check.
	if !driverConfig.EnableIPTables {
		return errors.New("Cannot program chains, EnableIPTable is disabled")
	}

	maskedAddrv4 := &net.IPNet{
		IP:   i.bridgeIPv4.IP.Mask(i.bridgeIPv4.Mask),
		Mask: i.bridgeIPv4.Mask,
	}
	return n.setupFirewallTables(iptables.IPv4, maskedAddrv4, config, i)
}

func (n *bridgeNetwork) setupIP6Tables(config *networkConfiguration, i *bridgeInterface) error {
	d := n.driver
	d.Lock()
	driverConfig := d.config
	d.Unlock()

	// Sanity check.
	if !driverConfig.EnableIP6Tables {
		return errors.New("Cannot program chains, EnableIP6Tables is disabled")
	}

	maskedAddrv6 := &net.IPNet{
		IP:   i.bridgeIPv6.IP.Mask(i.bridgeIPv6.Mask),
		Mask: i.bridgeIPv6.Mask,
	}

	return n.setupFirewallTables(iptables.IPv6, maskedAddrv6, config, i)
}

func (n *bridgeNetwork) setupNFTables(config *networkConfiguration, i *bridgeInterface) error {
	d := n.driver
	d.Lock()
	driverConfig := d.config
	d.Unlock()

	// Sanity check.
	if !driverConfig.EnableNFTables {
		return errors.New("Cannot program chains, EnableNFTables is disabled")
	}

	maskedAddrv4 := &net.IPNet{
		IP:   i.bridgeIPv4.IP.Mask(i.bridgeIPv4.Mask),
		Mask: i.bridgeIPv4.Mask,
	}
	return n.setupFirewallTables(nftables.IPv4, maskedAddrv4, config, i)
}

func (n *bridgeNetwork) setupNF6Tables(config *networkConfiguration, i *bridgeInterface) error {
	d := n.driver
	d.Lock()
	driverConfig := d.config
	d.Unlock()

	// Sanity check.
	if !driverConfig.EnableNFTables {
		return errors.New("Cannot program chains for ipv6, EnableNFTables is disabled")
	}

	maskedAddrv6 := &net.IPNet{
		IP:   i.bridgeIPv6.IP.Mask(i.bridgeIPv6.Mask),
		Mask: i.bridgeIPv6.Mask,
	}

	return n.setupFirewallTables(nftables.IPv6, maskedAddrv6, config, i)
}

func (n *bridgeNetwork) setupFirewallTables(ipVersion firewallapi.IPVersion, maskedAddr *net.IPNet, config *networkConfiguration, i *bridgeInterface) error {
	var err error

	d := n.driver
	d.Lock()
	driverConfig := d.config
	d.Unlock()

	// Pickup this configuration option from driver
	hairpinMode := !driverConfig.EnableUserlandProxy

	var table firewallapi.FirewallTable

	if driverConfig.EnableNFTables {
		table = nftables.GetTable(nftables.IPVersion(ipVersion))
	} else {
		table = iptables.GetTable(iptables.IPVersion(ipVersion))
	}

	if config.Internal {
		if err = setupInternalNetworkRules(config.BridgeName, maskedAddr, config.EnableICC, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupInternalNetworkRules(config.BridgeName, maskedAddr, config.EnableICC, false)
		})
	} else {
		if err = setupIPTablesInternal(config.HostIP, config.BridgeName, maskedAddr, config.EnableICC, config.EnableIPMasquerade, hairpinMode, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupIPTablesInternal(config.HostIP, config.BridgeName, maskedAddr, config.EnableICC, config.EnableIPMasquerade, hairpinMode, false)
		})
		natChain, filterChain, _, _, err := n.getDriverChains(ipVersion)
		if err != nil {
			return fmt.Errorf("Failed to setup IP tables, cannot acquire chain info %s", err.Error())
		}

		err = table.ProgramChain(natChain, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program NAT chain: %s", err.Error())
		}

		err = table.ProgramChain(filterChain, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program FILTER chain: %s", err.Error())
		}

		n.registerIptCleanFunc(func() error {
			return table.ProgramChain(filterChain, config.BridgeName, hairpinMode, false)
		})

		if ipVersion == iptables.IPv4 {
			n.portMapper.SetFirewallTablesChain(natChain, n.getNetworkBridgeName())
		} else {
			n.portMapperV6.SetFirewallTablesChain(natChain, n.getNetworkBridgeName())
		}
	}

	d.Lock()
	err = table.EnsureJumpRule("FORWARD", IsolationChain1)
	d.Unlock()
	return err
}

type firewallRule struct {
	table   firewallapi.Table
	chain   string
	preArgs []string
	args    []string
}

func setupIPTablesInternal(hostIP net.IP, bridgeIface string, addr *net.IPNet, icc, ipmasq, hairpin, enable bool) error {

	var (
		address   = addr.String()
		skipDNAT  = firewallRule{table: firewallapi.Nat, chain: DockerChain, preArgs: []string{"-t", "nat"}, args: []string{"-i", bridgeIface, "-j", "RETURN"}}
		outRule   = firewallRule{table: firewallapi.Filter, chain: "FORWARD", args: []string{"-i", bridgeIface, "!", "-o", bridgeIface, "-j", "ACCEPT"}}
		natArgs   []string
		hpNatArgs []string
	)
	// if hostIP is set use this address as the src-ip during SNAT
	if hostIP != nil {
		hostAddr := hostIP.String()
		natArgs = []string{"-s", address, "!", "-o", bridgeIface, "-j", "SNAT", "--to-source", hostAddr}
		hpNatArgs = []string{"-m", "addrtype", "--src-type", "LOCAL", "-o", bridgeIface, "-j", "SNAT", "--to-source", hostAddr}
		// Else use MASQUERADE which picks the src-ip based on NH from the route table
	} else {
		natArgs = []string{"-s", address, "!", "-o", bridgeIface, "-j", "MASQUERADE"}
		hpNatArgs = []string{"-m", "addrtype", "--src-type", "LOCAL", "-o", bridgeIface, "-j", "MASQUERADE"}
	}

	natRule := firewallRule{table: iptables.Nat, chain: "POSTROUTING", preArgs: []string{"-t", "nat"}, args: natArgs}
	hpNatRule := firewallRule{table: iptables.Nat, chain: "POSTROUTING", preArgs: []string{"-t", "nat"}, args: hpNatArgs}

	ipVersion := iptables.IPv4

	if addr.IP.To4() == nil {
		ipVersion = iptables.IPv6
	}

	// Set NAT.
	if ipmasq {
		if err := programChainRule(ipVersion, natRule, "NAT", enable); err != nil {
			return err
		}
	}

	if ipmasq && !hairpin {
		if err := programChainRule(ipVersion, skipDNAT, "SKIP DNAT", enable); err != nil {
			return err
		}
	}

	// In hairpin mode, masquerade traffic from localhost
	if hairpin {
		if err := programChainRule(ipVersion, hpNatRule, "MASQ LOCAL HOST", enable); err != nil {
			return err
		}
	}

	// Set Inter Container Communication.
	if err := setIcc(ipVersion, bridgeIface, icc, enable); err != nil {
		return err
	}

	// Set Accept on all non-intercontainer outgoing packets.
	return programChainRule(ipVersion, outRule, "ACCEPT NON_ICC OUTGOING", enable)
}

func programChainRule(version iptables.IPVersion, rule firewallRule, ruleDescr string, insert bool) error {

	iptable := iptables.GetTable(version)

	var (
		prefix    []string
		operation string
		condition bool
		doesExist = iptable.Exists(rule.table, rule.chain, rule.args...)
	)

	if insert {
		condition = !doesExist
		prefix = []string{"-I", rule.chain}
		operation = "enable"
	} else {
		condition = doesExist
		prefix = []string{"-D", rule.chain}
		operation = "disable"
	}
	if rule.preArgs != nil {
		prefix = append(rule.preArgs, prefix...)
	}

	if condition {
		if err := iptable.RawCombinedOutput(append(prefix, rule.args...)...); err != nil {
			return fmt.Errorf("Unable to %s %s rule: %s", operation, ruleDescr, err.Error())
		}
	}

	return nil
}

func setIcc(version iptables.IPVersion, bridgeIface string, iccEnable, insert bool) error {
	iptable := iptables.GetTable(version)
	var (
		table      = iptables.Filter
		chain      = "FORWARD"
		args       = []string{"-i", bridgeIface, "-o", bridgeIface, "-j"}
		acceptArgs = append(args, "ACCEPT")
		dropArgs   = append(args, "DROP")
	)

	if insert {
		if !iccEnable {
			iptable.Raw(append([]string{"-D", chain}, acceptArgs...)...)

			if !iptable.Exists(table, chain, dropArgs...) {
				if err := iptable.RawCombinedOutput(append([]string{"-A", chain}, dropArgs...)...); err != nil {
					return fmt.Errorf("Unable to prevent intercontainer communication: %s", err.Error())
				}
			}
		} else {
			iptable.Raw(append([]string{"-D", chain}, dropArgs...)...)

			if !iptable.Exists(table, chain, acceptArgs...) {
				if err := iptable.RawCombinedOutput(append([]string{"-I", chain}, acceptArgs...)...); err != nil {
					return fmt.Errorf("Unable to allow intercontainer communication: %s", err.Error())
				}
			}
		}
	} else {
		// Remove any ICC rule.
		if !iccEnable {
			if iptable.Exists(table, chain, dropArgs...) {
				iptable.Raw(append([]string{"-D", chain}, dropArgs...)...)
			}
		} else {
			if iptable.Exists(table, chain, acceptArgs...) {
				iptable.Raw(append([]string{"-D", chain}, acceptArgs...)...)
			}
		}
	}

	return nil
}

// Control Inter Network Communication. Install[Remove] only if it is [not] present.
func setINC(version iptables.IPVersion, iface string, enable bool) error {
	iptable := iptables.GetTable(version)
	var (
		action    = iptables.Insert
		actionMsg = "add"
		chains    = []string{IsolationChain1, IsolationChain2}
		rules     = [][]string{
			{"-i", iface, "!", "-o", iface, "-j", IsolationChain2},
			{"-o", iface, "-j", "DROP"},
		}
	)

	if !enable {
		action = iptables.Delete
		actionMsg = "remove"
	}

	for i, chain := range chains {
		if err := iptable.ProgramRule(iptables.Filter, chain, action, rules[i]); err != nil {
			msg := fmt.Sprintf("unable to %s inter-network communication rule: %v", actionMsg, err)
			if enable {
				if i == 1 {
					// Rollback the rule installed on first chain
					if err2 := iptable.ProgramRule(iptables.Filter, chains[0], iptables.Delete, rules[0]); err2 != nil {
						logrus.Warnf("Failed to rollback iptables rule after failure (%v): %v", err, err2)
					}
				}
				return fmt.Errorf(msg)
			}
			logrus.Warn(msg)
		}
	}

	return nil
}

// Obsolete chain from previous docker versions
const oldIsolationChain = "DOCKER-ISOLATION"

func removeIPChains(version iptables.IPVersion) {
	ipt := iptables.IPTable{Version: version}

	// Remove obsolete rules from default chains
	ipt.ProgramRule(iptables.Filter, "FORWARD", iptables.Delete, []string{"-j", oldIsolationChain})

	// Remove chains
	for _, chainInfo := range []iptables.ChainInfo{
		{Name: DockerChain, Table: iptables.Nat, FirewallTable: ipt},
		{Name: DockerChain, Table: iptables.Filter, FirewallTable: ipt},
		{Name: IsolationChain1, Table: iptables.Filter, FirewallTable: ipt},
		{Name: IsolationChain2, Table: iptables.Filter, FirewallTable: ipt},
		{Name: oldIsolationChain, Table: iptables.Filter, FirewallTable: ipt},
	} {

		if err := chainInfo.Remove(); err != nil {
			logrus.Warnf("Failed to remove existing iptables entries in table %s chain %s : %v", chainInfo.Table, chainInfo.Name, err)
		}
	}
}

func setupInternalNetworkRules(bridgeIface string, addr *net.IPNet, icc, insert bool) error {
	var (
		inDropRule  = firewallRule{table: iptables.Filter, chain: IsolationChain1, args: []string{"-i", bridgeIface, "!", "-d", addr.String(), "-j", "DROP"}}
		outDropRule = firewallRule{table: iptables.Filter, chain: IsolationChain1, args: []string{"-o", bridgeIface, "!", "-s", addr.String(), "-j", "DROP"}}
	)

	version := iptables.IPv4

	if addr.IP.To4() == nil {
		version = iptables.IPv6
	}

	if err := programChainRule(version, inDropRule, "DROP INCOMING", insert); err != nil {
		return err
	}
	if err := programChainRule(version, outDropRule, "DROP OUTGOING", insert); err != nil {
		return err
	}
	// Set Inter Container Communication.
	return setIcc(version, bridgeIface, icc, insert)
}

func clearEndpointConnections(nlh *netlink.Handle, ep *bridgeEndpoint) {
	var ipv4List []net.IP
	var ipv6List []net.IP
	if ep.addr != nil {
		ipv4List = append(ipv4List, ep.addr.IP)
	}
	if ep.addrv6 != nil {
		ipv6List = append(ipv6List, ep.addrv6.IP)
	}
	iptables.DeleteConntrackEntries(nlh, ipv4List, ipv6List)
}
