package bridge

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/docker/libnetwork/iptables"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// DockerChain: DOCKER iptable chain name
const (
	DockerChain    = "DOCKER"
	IsolationChain = "DOCKER-ISOLATION"
)

func setupIPChains(config *configuration, version iptables.IPVersion) (*iptables.ChainInfo, *iptables.ChainInfo, *iptables.ChainInfo, error) {
	// Sanity check.
	if config.EnableIPTables == false {
		return nil, nil, nil, errors.New("cannot create new chains, EnableIPTable is disabled")
	}

	hairpinMode := !config.EnableUserlandProxy

	iptable := iptables.GetIptable(version)

	natChain, err := iptable.NewChain(DockerChain, iptables.Nat, hairpinMode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create NAT chain: %v", err)
	}
	defer func() {
		if err != nil {
			if err := iptable.RemoveExistingChain(DockerChain, iptables.Nat); err != nil {
				logrus.Warnf("failed on removing iptables NAT chain on cleanup: %v", err)
			}
		}
	}()

	filterChain, err := iptable.NewChain(DockerChain, iptables.Filter, false)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create FILTER chain: %v", err)
	}
	defer func() {
		if err != nil {
			if err := iptable.RemoveExistingChain(DockerChain, iptables.Filter); err != nil {
				logrus.Warnf("failed on removing iptables FILTER chain on cleanup: %v", err)
			}
		}
	}()

	isolationChain, err := iptable.NewChain(IsolationChain, iptables.Filter, false)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create FILTER isolation chain: %v", err)
	}

	if err := iptable.AddReturnRule(IsolationChain); err != nil {
		return nil, nil, nil, err
	}

	return natChain, filterChain, isolationChain, nil
}

func (n *bridgeNetwork) setupIPTables(config *networkConfiguration, i *bridgeInterface) error {
	var err error

	d := n.driver
	d.Lock()
	driverConfig := d.config
	d.Unlock()

	// Sanity check.
	if driverConfig.EnableIPTables == false {
		return errors.New("Cannot program chains, EnableIPTable is disabled")
	}

	// Pickup this configuration option from driver
	hairpinMode := !driverConfig.EnableUserlandProxy

	maskedAddrv4 := &net.IPNet{
		IP:   i.bridgeIPv4.IP.Mask(i.bridgeIPv4.Mask),
		Mask: i.bridgeIPv4.Mask,
	}

	iptable := iptables.GetIptable(iptables.IPv4)

	if config.Internal {
		if err = setupInternalNetworkRules(config.BridgeName, maskedAddrv4, config.EnableICC, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupInternalNetworkRules(config.BridgeName, maskedAddrv4, config.EnableICC, false)
		})
	} else {
		if err = setupIPTablesInternal(config.BridgeName, maskedAddrv4, config.EnableICC, config.EnableIPMasquerade, hairpinMode, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupIPTablesInternal(config.BridgeName, maskedAddrv4, config.EnableICC, config.EnableIPMasquerade, hairpinMode, false)
		})
		natChain, filterChain, _, err := n.getDriverChains(iptables.IPv4)
		if err != nil {
			return fmt.Errorf("Failed to setup IP tables, cannot acquire chain info %s", err.Error())
		}

		err = iptable.ProgramChain(natChain, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program NAT chain: %s", err.Error())
		}

		err = iptable.ProgramChain(filterChain, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program FILTER chain: %s", err.Error())
		}

		n.registerIptCleanFunc(func() error {
			return iptable.ProgramChain(filterChain, config.BridgeName, hairpinMode, false)
		})

		n.portMapper.SetIptablesChain(natChain, n.getNetworkBridgeName())
	}

	d.Lock()
	err = iptable.EnsureJumpRule("FORWARD", IsolationChain)
	d.Unlock()
	if err != nil {
		return err
	}

	if !driverConfig.EnableIPv6 || i.bridgeIPv6 == nil {
		return nil
	}

	maskedAddrv6 := &net.IPNet{
		IP:   i.bridgeIPv6.IP.Mask(i.bridgeIPv6.Mask),
		Mask: i.bridgeIPv6.Mask,
	}

	iptable = iptables.GetIptable(iptables.IPv6)

	if config.Internal {
		if err = setupInternalNetworkRules(config.BridgeName, maskedAddrv6, config.EnableICC, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupInternalNetworkRules(config.BridgeName, maskedAddrv6, config.EnableICC, false)
		})
	} else {
		if err = setupIPTablesInternal(config.BridgeName, maskedAddrv6, config.EnableICC, config.EnableIPMasquerade, hairpinMode, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupIPTablesInternal(config.BridgeName, maskedAddrv6, config.EnableICC, config.EnableIPMasquerade, hairpinMode, false)
		})
		natChainV6, filterChainV6, _, err := n.getDriverChains(iptables.IPv6)
		if err != nil {
			return fmt.Errorf("Failed to setup IP tables, cannot acquire chain info %s", err.Error())
		}

		err = iptable.ProgramChain(natChainV6, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program NAT chain: %s", err.Error())
		}

		err = iptable.ProgramChain(filterChainV6, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program FILTER chain: %s", err.Error())
		}

		n.registerIptCleanFunc(func() error {
			return iptable.ProgramChain(filterChainV6, config.BridgeName, hairpinMode, false)
		})

		n.portMapperV6.SetIptablesChain(natChainV6, n.getNetworkBridgeName())
	}

	d.Lock()
	err = iptable.EnsureJumpRule("FORWARD", IsolationChain)
	d.Unlock()
	if err != nil {
		return err
	}

	return nil
}

type iptRule struct {
	table   iptables.Table
	chain   string
	preArgs []string
	args    []string
}

func setupIPTablesInternal(bridgeIface string, addr net.Addr, icc, ipmasq, hairpin, enable bool) error {

	var (
		address   = addr.String()
		natRule   = iptRule{table: iptables.Nat, chain: "POSTROUTING", preArgs: []string{"-t", "nat"}, args: []string{"-s", address, "!", "-o", bridgeIface, "-j", "MASQUERADE"}}
		hpNatRule = iptRule{table: iptables.Nat, chain: "POSTROUTING", preArgs: []string{"-t", "nat"}, args: []string{"-m", "addrtype", "--src-type", "LOCAL", "-o", bridgeIface, "-j", "MASQUERADE"}}
		skipDNAT  = iptRule{table: iptables.Nat, chain: DockerChain, preArgs: []string{"-t", "nat"}, args: []string{"-i", bridgeIface, "-j", "RETURN"}}
		outRule   = iptRule{table: iptables.Filter, chain: "FORWARD", args: []string{"-i", bridgeIface, "!", "-o", bridgeIface, "-j", "ACCEPT"}}
	)

	ipVersion := iptables.IPv4

	if strings.Contains(address, ":") {
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
	if err := programChainRule(ipVersion, outRule, "ACCEPT NON_ICC OUTGOING", enable); err != nil {
		return err
	}

	return nil
}

func programChainRule(version iptables.IPVersion, rule iptRule, ruleDescr string, insert bool) error {

	iptable := iptables.GetIptable(version)

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
	iptable := iptables.GetIptable(version)
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

// Control Inter Network Communication. Install/remove only if it is not/is present.
func setINC(version iptables.IPVersion, iface1, iface2 string, enable bool) error {
	iptable := iptables.GetIptable(version)
	var (
		table = iptables.Filter
		chain = IsolationChain
		args  = [2][]string{{"-i", iface1, "-o", iface2, "-j", "DROP"}, {"-i", iface2, "-o", iface1, "-j", "DROP"}}
	)

	if enable {
		for i := 0; i < 2; i++ {
			if iptable.Exists(table, chain, args[i]...) {
				continue
			}
			if err := iptable.RawCombinedOutput(append([]string{"-I", chain}, args[i]...)...); err != nil {
				return fmt.Errorf("unable to add inter-network communication rule: %v", err)
			}
		}
	} else {
		for i := 0; i < 2; i++ {
			if !iptable.Exists(table, chain, args[i]...) {
				continue
			}
			if err := iptable.RawCombinedOutput(append([]string{"-D", chain}, args[i]...)...); err != nil {
				return fmt.Errorf("unable to remove inter-network communication rule: %v", err)
			}
		}
	}

	return nil
}

func removeIPChains() {
	for _, chainInfo := range []iptables.ChainInfo{
		{Name: DockerChain, Table: iptables.Nat},
		{Name: DockerChain, Table: iptables.Filter},
		{Name: IsolationChain, Table: iptables.Filter},
	} {
		if err := chainInfo.Remove(); err != nil {
			logrus.Warnf("Failed to remove existing iptables entries in table %s chain %s : %v", chainInfo.Table, chainInfo.Name, err)
		}
	}
}

func setupInternalNetworkRules(bridgeIface string, addr net.Addr, icc, insert bool) error {
	var (
		inDropRule  = iptRule{table: iptables.Filter, chain: IsolationChain, args: []string{"-i", bridgeIface, "!", "-d", addr.String(), "-j", "DROP"}}
		outDropRule = iptRule{table: iptables.Filter, chain: IsolationChain, args: []string{"-o", bridgeIface, "!", "-s", addr.String(), "-j", "DROP"}}
	)

	version := iptables.IPv4

	if strings.Contains(addr.String(), ":") {
		version = iptables.IPv6
	}

	if err := programChainRule(version, inDropRule, "DROP INCOMING", insert); err != nil {
		return err
	}
	if err := programChainRule(version, outDropRule, "DROP OUTGOING", insert); err != nil {
		return err
	}
	// Set Inter Container Communication.
	if err := setIcc(version, bridgeIface, icc, insert); err != nil {
		return err
	}
	return nil
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
