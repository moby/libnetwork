package main

import (
	"fmt"
	"log"
	"net"
	"runtime"

	"github.com/docker/libnetwork/iptables"
	"github.com/vishvananda/netns"
)

const (
	// outputChain used for docker embed dns
	outputChain = "DOCKER_OUTPUT"
	//postroutingchain used for docker embed dns
	postroutingchain = "DOCKER_POSTROUTING"
	dnsPort          = "53"
)

// SetupResolver programs the resolver rules
func setupResolver(path, localAddress, localTCPAddress string) (int, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	resolverIP, ipPort, _ := net.SplitHostPort(localAddress)
	_, tcpPort, _ := net.SplitHostPort(localTCPAddress)
	rules := [][]string{
		{"-t", "nat", "-I", outputChain, "-d", resolverIP, "-p", "udp", "--dport", dnsPort, "-j", "DNAT", "--to-destination", localAddress},
		{"-t", "nat", "-I", postroutingchain, "-s", resolverIP, "-p", "udp", "--sport", ipPort, "-j", "SNAT", "--to-source", ":" + dnsPort},
		{"-t", "nat", "-I", outputChain, "-d", resolverIP, "-p", "tcp", "--dport", dnsPort, "-j", "DNAT", "--to-destination", localTCPAddress},
		{"-t", "nat", "-I", postroutingchain, "-s", resolverIP, "-p", "tcp", "--sport", tcpPort, "-j", "SNAT", "--to-source", ":" + dnsPort},
	}

	ns, err := netns.GetFromPath(path)
	if err != nil {
		return 2, fmt.Errorf("failed get network namespace %q: %v", path, err)
	}
	defer ns.Close()

	if err := netns.Set(ns); err != nil {
		return 3, fmt.Errorf("setting into container net ns %v failed, %v", path, err)
	}

	// insert outputChain and postroutingchain
	err = iptables.RawCombinedOutputNative("-t", "nat", "-C", "OUTPUT", "-d", resolverIP, "-j", outputChain)
	if err == nil {
		iptables.RawCombinedOutputNative("-t", "nat", "-F", outputChain)
	} else {
		iptables.RawCombinedOutputNative("-t", "nat", "-N", outputChain)
		iptables.RawCombinedOutputNative("-t", "nat", "-I", "OUTPUT", "-d", resolverIP, "-j", outputChain)
	}

	err = iptables.RawCombinedOutputNative("-t", "nat", "-C", "POSTROUTING", "-d", resolverIP, "-j", postroutingchain)
	if err == nil {
		iptables.RawCombinedOutputNative("-t", "nat", "-F", postroutingchain)
	} else {
		iptables.RawCombinedOutputNative("-t", "nat", "-N", postroutingchain)
		iptables.RawCombinedOutputNative("-t", "nat", "-I", "POSTROUTING", "-d", resolverIP, "-j", postroutingchain)
	}

	for _, rule := range rules {
		if iptables.RawCombinedOutputNative(rule...) != nil {
			log.Printf("failed setting up resolver rule %v: %v", rule, err)
		}
	}

	return 0, nil
}
