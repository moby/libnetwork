package bridge

import (
	"errors"
	"fmt"
	"net"

	"github.com/docker/libnetwork/ip6tables"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// DockerChain: DOCKER iptable chain name
const (
	ip6tDockerChain = "DOCKER"
	// Isolation between bridge networks is achieved in two stages by means
	// of the following two chains in the filter table. The first chain matches
	// on the source interface being a bridge network's bridge and the
	// destination being a different interface. A positive match leads to the
	// second isolation chain. No match returns to the parent chain. The second
	// isolation chain matches on destination interface being a bridge network's
	// bridge. A positive match identifies a packet originated from one bridge
	// network's bridge destined to another bridge network's bridge and will
	// result in the packet being dropped. No match returns to the parent chain.
	ip6tIsolationChain1 = "DOCKER-ISOLATION-STAGE-1"
	ip6tIsolationChain2 = "DOCKER-ISOLATION-STAGE-2"
)

func setupIP6Chains(config *configuration) (*ip6tables.ChainInfo, *ip6tables.ChainInfo, *ip6tables.ChainInfo, *ip6tables.ChainInfo, error) {
	// Sanity check.
	if config.EnableIP6Tables == false {
		return nil, nil, nil, nil, errors.New("cannot create new chains, EnableIP6Table is disabled")
	}

	hairpinMode := !config.EnableUserlandProxy

	natChain, err := ip6tables.NewChain(ip6tDockerChain, ip6tables.Nat, hairpinMode)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create NAT chain %s: %v", ip6tDockerChain, err)
	}
	defer func() {
		if err != nil {
			if err := ip6tables.RemoveExistingChain(ip6tDockerChain, ip6tables.Nat); err != nil {
				logrus.Warnf("failed on removing ip6tables NAT chain %s on cleanup: %v", ip6tDockerChain, err)
			}
		}
	}()

	filterChain, err := ip6tables.NewChain(ip6tDockerChain, ip6tables.Filter, false)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create FILTER chain %s: %v", ip6tDockerChain, err)
	}
	defer func() {
		if err != nil {
			if err := ip6tables.RemoveExistingChain(ip6tDockerChain, ip6tables.Filter); err != nil {
				logrus.Warnf("failed on removing ip6tables FILTER chain %s on cleanup: %v", ip6tDockerChain, err)
			}
		}
	}()

	isolationChain1, err := ip6tables.NewChain(ip6tIsolationChain1, ip6tables.Filter, false)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create FILTER isolation chain: %v", err)
	}
	defer func() {
		if err != nil {
			if err := ip6tables.RemoveExistingChain(ip6tIsolationChain1, ip6tables.Filter); err != nil {
				logrus.Warnf("failed on removing ip6tables FILTER chain %s on cleanup: %v", ip6tIsolationChain1, err)
			}
		}
	}()

	isolationChain2, err := ip6tables.NewChain(ip6tIsolationChain2, ip6tables.Filter, false)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create FILTER isolation chain: %v", err)
	}
	defer func() {
		if err != nil {
			if err := ip6tables.RemoveExistingChain(ip6tIsolationChain2, ip6tables.Filter); err != nil {
				logrus.Warnf("failed on removing ip6tables FILTER chain %s on cleanup: %v", ip6tIsolationChain2, err)
			}
		}
	}()

	if err := ip6tables.AddReturnRule(ip6tIsolationChain1); err != nil {
		return nil, nil, nil, nil, err
	}

	if err := ip6tables.AddReturnRule(ip6tIsolationChain2); err != nil {
		return nil, nil, nil, nil, err
	}

	return natChain, filterChain, isolationChain1, isolationChain2, nil
}

func (n *bridgeNetwork) setupIP6Tables(config *networkConfiguration, i *bridgeInterface) error {
	var err error

	d := n.driver
	d.Lock()
	driverConfig := d.config
	d.Unlock()

	// Sanity check.
	if driverConfig.EnableIP6Tables == false {
		return errors.New("Cannot program chains, EnableIP6Table is disabled")
	}

	// Pickup this configuration option from driver
	hairpinMode := !driverConfig.EnableUserlandProxy

	maskedAddrv6 := &net.IPNet{
		IP:   i.bridgeIPv6.IP.Mask(i.bridgeIPv6.Mask),
		Mask: i.bridgeIPv6.Mask,
	}
	if config.Internal {
		if err = setupInternalNetworkRules(config.BridgeName, maskedAddrv6, config.EnableICC, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupInternalNetworkRules(config.BridgeName, maskedAddrv6, config.EnableICC, false)
		})
	} else {
		if err = setupIP6TablesInternal(config.BridgeName, maskedAddrv6, config.EnableICC, config.EnableIPMasquerade, hairpinMode, true); err != nil {
			return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
		}
		n.registerIptCleanFunc(func() error {
			return setupIP6TablesInternal(config.BridgeName, maskedAddrv6, config.EnableICC, config.EnableIPMasquerade, hairpinMode, false)
		})
		ip6tNatChain, ip6tFilterChain, _, _, err := n.getIP6DriverChains()
		if err != nil {
			return fmt.Errorf("Failed to setup IP tables, cannot acquire chain info %s", err.Error())
		}

		err = ip6tables.ProgramChain(ip6tNatChain, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program NAT chain: %s", err.Error())
		}

		err = ip6tables.ProgramChain(ip6tFilterChain, config.BridgeName, hairpinMode, true)
		if err != nil {
			return fmt.Errorf("Failed to program FILTER chain: %s", err.Error())
		}

		n.registerIP6tCleanFunc(func() error {
			return ip6tables.ProgramChain(ip6tFilterChain, config.BridgeName, hairpinMode, false)
		})

		n.portMapper.SetIP6tablesChain(ip6tNatChain, n.getNetworkBridgeName())
	}

	d.Lock()
	err = ip6tables.EnsureJumpRule("FORWARD", ip6tIsolationChain1)
	d.Unlock()
	if err != nil {
		return err
	}

	return nil
}

type ip6tRule struct {
	table   ip6tables.Table
	chain   string
	preArgs []string
	args    []string
}

func setupIP6TablesInternal(bridgeIface string, addr net.Addr, icc, ipmasq, hairpin, enable bool) error {

	var (
		address   = addr.String()
		natRule   = ip6tRule{table: ip6tables.Nat, chain: "POSTROUTING", preArgs: []string{"-t", "nat"}, args: []string{"-s", address, "!", "-o", bridgeIface, "-j", "MASQUERADE"}}
		hpNatRule = ip6tRule{table: ip6tables.Nat, chain: "POSTROUTING", preArgs: []string{"-t", "nat"}, args: []string{"-m", "addrtype", "--src-type", "LOCAL", "-o", bridgeIface, "-j", "MASQUERADE"}}
		skipDNAT  = ip6tRule{table: ip6tables.Nat, chain: ip6tDockerChain, preArgs: []string{"-t", "nat"}, args: []string{"-i", bridgeIface, "-j", "RETURN"}}
		outRule   = ip6tRule{table: ip6tables.Filter, chain: "FORWARD", args: []string{"-i", bridgeIface, "!", "-o", bridgeIface, "-j", "ACCEPT"}}
	)

	// Set NAT.
	if ipmasq {
		if err := programIP6ChainRule(natRule, "NAT", enable); err != nil {
			return err
		}
	}

	if ipmasq && !hairpin {
		if err := programIP6ChainRule(skipDNAT, "SKIP DNAT", enable); err != nil {
			return err
		}
	}

	// In hairpin mode, masquerade traffic from localhost
	if hairpin {
		if err := programIP6ChainRule(hpNatRule, "MASQ LOCAL HOST", enable); err != nil {
			return err
		}
	}

	// Set Inter Container Communication.
	if err := setIP6Icc(bridgeIface, icc, enable); err != nil {
		return err
	}

	// Set Accept on all non-intercontainer outgoing packets.
	return programIP6ChainRule(outRule, "ACCEPT NON_ICC OUTGOING", enable)
}

func programIP6ChainRule(rule ip6tRule, ruleDescr string, insert bool) error {
	var (
		prefix    []string
		operation string
		condition bool
		doesExist = ip6tables.Exists(rule.table, rule.chain, rule.args...)
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
		if err := ip6tables.RawCombinedOutput(append(prefix, rule.args...)...); err != nil {
			return fmt.Errorf("Unable to %s %s rule: %s", operation, ruleDescr, err.Error())
		}
	}

	return nil
}

func setIP6Icc(bridgeIface string, iccEnable, insert bool) error {
	var (
		table      = ip6tables.Filter
		chain      = "FORWARD"
		args       = []string{"-i", bridgeIface, "-o", bridgeIface, "-j"}
		acceptArgs = append(args, "ACCEPT")
		dropArgs   = append(args, "DROP")
	)

	if insert {
		if !iccEnable {
			ip6tables.Raw(append([]string{"-D", chain}, acceptArgs...)...)

			if !ip6tables.Exists(table, chain, dropArgs...) {
				if err := ip6tables.RawCombinedOutput(append([]string{"-A", chain}, dropArgs...)...); err != nil {
					return fmt.Errorf("Unable to prevent intercontainer communication: %s", err.Error())
				}
			}
		} else {
			ip6tables.Raw(append([]string{"-D", chain}, dropArgs...)...)

			if !ip6tables.Exists(table, chain, acceptArgs...) {
				if err := ip6tables.RawCombinedOutput(append([]string{"-I", chain}, acceptArgs...)...); err != nil {
					return fmt.Errorf("Unable to allow intercontainer communication: %s", err.Error())
				}
			}
		}
	} else {
		// Remove any ICC rule.
		if !iccEnable {
			if ip6tables.Exists(table, chain, dropArgs...) {
				ip6tables.Raw(append([]string{"-D", chain}, dropArgs...)...)
			}
		} else {
			if ip6tables.Exists(table, chain, acceptArgs...) {
				ip6tables.Raw(append([]string{"-D", chain}, acceptArgs...)...)
			}
		}
	}

	return nil
}

// Control Inter Network Communication. Install[Remove] only if it is [not] present.
func setIP6INC(iface string, enable bool) error {
	var (
		action    = ip6tables.Insert
		actionMsg = "add"
		chains    = []string{IsolationChain1, IsolationChain2}
		rules     = [][]string{
			{"-i", iface, "!", "-o", iface, "-j", IsolationChain2},
			{"-o", iface, "-j", "DROP"},
		}
	)

	if !enable {
		action = ip6tables.Delete
		actionMsg = "remove"
	}

	for i, chain := range chains {
		if err := ip6tables.ProgramRule(ip6tables.Filter, chain, action, rules[i]); err != nil {
			msg := fmt.Sprintf("unable to %s inter-network communication rule: %v", actionMsg, err)
			if enable {
				if i == 1 {
					// Rollback the rule installed on first chain
					if err2 := ip6tables.ProgramRule(ip6tables.Filter, chains[0], ip6tables.Delete, rules[0]); err2 != nil {
						//logrus.Warn("Failed to rollback ip6tables rule after failure (%v): %v", err, err2)
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
const oldIP6IsolationChain = "DOCKER-ISOLATION"

func removeIP6Chains() {
	// Remove obsolete rules from default chains
	ip6tables.ProgramRule(ip6tables.Filter, "FORWARD", ip6tables.Delete, []string{"-j", oldIsolationChain})

	// Remove chains
	for _, chainInfo := range []ip6tables.ChainInfo{
		{Name: DockerChain, Table: ip6tables.Nat},
		{Name: DockerChain, Table: ip6tables.Filter},
		{Name: IsolationChain1, Table: ip6tables.Filter},
		{Name: IsolationChain2, Table: ip6tables.Filter},
		{Name: oldIsolationChain, Table: ip6tables.Filter},
	} {
		if err := chainInfo.Remove(); err != nil {
			logrus.Warnf("Failed to remove existing ip6tables entries in table %s chain %s : %v", chainInfo.Table, chainInfo.Name, err)
		}
	}
}

func setupIP6InternalNetworkRules(bridgeIface string, addr net.Addr, icc, insert bool) error {
	var (
		inDropRule  = ip6tRule{table: ip6tables.Filter, chain: IsolationChain1, args: []string{"-i", bridgeIface, "!", "-d", addr.String(), "-j", "DROP"}}
		outDropRule = ip6tRule{table: ip6tables.Filter, chain: IsolationChain1, args: []string{"-o", bridgeIface, "!", "-s", addr.String(), "-j", "DROP"}}
	)
	if err := programIP6ChainRule(inDropRule, "DROP INCOMING", insert); err != nil {
		return err
	}
	if err := programIP6ChainRule(outDropRule, "DROP OUTGOING", insert); err != nil {
		return err
	}
	// Set Inter Container Communication.
	return setIcc(bridgeIface, icc, insert)
}

func clearIP6EndpointConnections(nlh *netlink.Handle, ep *bridgeEndpoint) {
	var ipv4List []net.IP
	var ipv6List []net.IP
	if ep.addr != nil {
		ipv4List = append(ipv4List, ep.addr.IP)
	}
	if ep.addrv6 != nil {
		ipv6List = append(ipv6List, ep.addrv6.IP)
	}
	ip6tables.DeleteConntrackEntries(nlh, ipv4List, ipv6List)
}
