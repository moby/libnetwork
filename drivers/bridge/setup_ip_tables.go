package bridge

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/daemon/networkdriver/portmapper"
	"github.com/docker/docker/pkg/iptables"
	"net"
	"strings"
)

const (
	DOCKER_CHAIN = "DOCKER"
)

func SetupIPTables(i *Interface) error {
	// Sanity check
	if i.Config.EnableIPTables == false {
		return fmt.Errorf("Unexpected request to set IP tables for interface: %s", i.Config.BridgeName)
	}

	addrv4, _, err := i.Addresses()
	if err != nil {
		return fmt.Errorf("Failed to setup IP tables, cannot acquire Interface address: %s", err.Error())
	}
	if err = setupIPTables(i.Config.BridgeName, addrv4, i.Config.EnableICC, i.Config.EnableIPMasquerade, true); err != nil {
		return fmt.Errorf("Failed to Setup IP tables: %s", err.Error())
	}

	_, err = iptables.NewChain(DOCKER_CHAIN, i.Config.BridgeName, iptables.Nat)
	if err != nil {
		return fmt.Errorf("Failed to create NAT chain: %s", err.Error())
	}

	chain, err := iptables.NewChain(DOCKER_CHAIN, i.Config.BridgeName, iptables.Filter)
	if err != nil {
		return fmt.Errorf("Failed to create FILTER chain: %s", err.Error())
	}

	portmapper.SetIptablesChain(chain)

	return nil
}

func setupIPTables(bridgeIface string, addr net.Addr, icc, ipmasq, enable bool) error {
	var (
		// Make sure we get only the network and not the name resolution
		address =  strings.Split(addr.String(), " ")[0]
		natArgs = []string{"POSTROUTING", "-t", "nat", "-s", address, "!", "-o", bridgeIface, "-j", "MASQUERADE"}
		outArgs = []string{"FORWARD", "-i", bridgeIface, "!", "-o", bridgeIface, "-j", "ACCEPT"}
		inArgs  = []string{"FORWARD", "-o", bridgeIface, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	)
	// Set NAT
	if ipmasq {
		if err := programIPTableEntry(natArgs, "POSTROUTING", enable); err != nil {
			return err
		}
	}

	// Set Inter Container Communication
	if err := setIcc(bridgeIface, icc, enable); err != nil {
		return err
	}

	// Set Accept on all non-intercontainer outgoing packets
	if err := programIPTableEntry(outArgs, "FORWARD outgoing", enable); err != nil {
		return err
	}

	// Set Accept on incoming packets for existing connections
	if err := programIPTableEntry(inArgs, "FORWARD incoming", enable); err != nil {
		return err
	}

	return nil
}

func programIPTableEntry(args []string, ruleDescr string, add bool) error {
	var (
		prefix    []string
		operation string
		condition bool
	)

	if add {
		condition = !iptables.Exists(args...)
		prefix = []string{"-I"}
		operation = "enable"
	} else {
		condition = iptables.Exists(args...)
		prefix = []string{"-D"}
		operation = "disable"
	}

	if condition {
		if output, err := iptables.Raw(append(prefix, args...)...); err != nil {
			return fmt.Errorf("Unable to %s %s rule: %s", operation, ruleDescr, err.Error())
		} else if len(output) != 0 {
			return &iptables.ChainError{Chain: ruleDescr, Output: output}
		}
	}

	return nil
}

func setIcc(bridgeIface string, iccEnable, add bool) error {
	var (
		args       = []string{"FORWARD", "-i", bridgeIface, "-o", bridgeIface, "-j"}
		acceptArgs = append(args, "ACCEPT")
		dropArgs   = append(args, "DROP")
	)

	if add {
		if !iccEnable {
			iptables.Raw(append([]string{"-D"}, acceptArgs...)...)

			if !iptables.Exists(dropArgs...) {
				log.Debugf("Disable inter-container communication")
				if output, err := iptables.Raw(append([]string{"-I"}, dropArgs...)...); err != nil {
					return fmt.Errorf("Unable to prevent intercontainer communication: %s", err.Error())
				} else if len(output) != 0 {
					return fmt.Errorf("Error disabling intercontainer communication: %s", output)
				}
			}
		} else {
			iptables.Raw(append([]string{"-D"}, dropArgs...)...)

			if !iptables.Exists(acceptArgs...) {
				log.Debugf("Enable inter-container communication")
				if output, err := iptables.Raw(append([]string{"-I"}, acceptArgs...)...); err != nil {
					return fmt.Errorf("Unable to allow intercontainer communication: %s", err.Error())
				} else if len(output) != 0 {
					return fmt.Errorf("Error enabling intercontainer communication: %s", output)
				}
			}
		}
	} else {
		// Remove any ICC rule
		if !iccEnable {
			if iptables.Exists(dropArgs...) {
				iptables.Raw(append([]string{"-D"}, dropArgs...)...)
			}
		} else {
			if iptables.Exists(acceptArgs...) {
				iptables.Raw(append([]string{"-D"}, acceptArgs...)...)
			}
		}
	}

	return nil
}
