package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/docker/libnetwork/iptables"
	"github.com/docker/libnetwork/types"
	"github.com/gogo/protobuf/proto"
	"github.com/vishvananda/netns"
)

func marker(path, vip, eip, fwMark, file string, isDelete bool) (int, error) {
	var (
		ingressPorts []*types.PortConfig
		err          error
	)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if file != "" {
		ingressPorts, err = readPortsFromFile(file)
		if err != nil {
			return 2, fmt.Errorf("failed reading ingress ports file: %v", err)
		}
	}

	addDelOpt := "-A"
	if isDelete {
		addDelOpt = "-D"
	}

	rules := [][]string{}
	for _, iPort := range ingressPorts {
		rule := strings.Fields(fmt.Sprintf("-t mangle %s PREROUTING -p %s --dport %d -j MARK --set-mark %s",
			addDelOpt, strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]), iPort.PublishedPort, fwMark))
		rules = append(rules, rule)
	}

	ns, err := netns.GetFromPath(path)
	if err != nil {
		return 3, fmt.Errorf("failed get network namespace %q: %v", path, err)
	}
	defer ns.Close()

	if err := netns.Set(ns); err != nil {
		return 4, fmt.Errorf("setting into container net ns %v failed, %v", path, err)
	}

	if addDelOpt == "-A" {
		eIP, subnet, err := net.ParseCIDR(eip)
		if err != nil {
			return 5, fmt.Errorf("failed to parse endpoint IP %s: %v", eip, err)
		}

		ruleParams := strings.Fields(fmt.Sprintf("-m ipvs --ipvs -d %s -j SNAT --to-source %s", subnet, eIP))
		if !iptables.Exists("nat", "POSTROUTING", ruleParams...) {
			rule := append(strings.Fields("-t nat -A POSTROUTING"), ruleParams...)
			rules = append(rules, rule)

			err := ioutil.WriteFile("/proc/sys/net/ipv4/vs/conntrack", []byte{'1', '\n'}, 0644)
			if err != nil {
				return 6, fmt.Errorf("failed to write to /proc/sys/net/ipv4/vs/conntrack: %v", err)
			}
		}
	}

	rule := strings.Fields(fmt.Sprintf("-t mangle %s OUTPUT -d %s/32 -j MARK --set-mark %s", addDelOpt, vip, fwMark))
	rules = append(rules, rule)

	rule = strings.Fields(fmt.Sprintf("-t nat %s OUTPUT -p icmp --icmp echo-request -d %s -j DNAT --to 127.0.0.1", addDelOpt, vip))
	rules = append(rules, rule)

	for _, rule := range rules {
		if err := iptables.RawCombinedOutputNative(rule...); err != nil {
			return 7, fmt.Errorf("setting up rule failed, %v: %v", rule, err)
		}
	}
	return 0, nil
}

func redirecter(path, eip, file string) (int, error) {
	var (
		ingressPorts []*types.PortConfig
		err          error
	)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if file != "" {
		ingressPorts, err = readPortsFromFile(file)
		if err != nil {
			return 2, fmt.Errorf("failed reading ingress ports file: %v", err)
		}
	}

	eIP, _, err := net.ParseCIDR(eip)
	if err != nil {
		return 3, fmt.Errorf("failed to parse endpoint IP %s: %v", eip, err)
	}

	rules := [][]string{}
	for _, iPort := range ingressPorts {
		rule := strings.Fields(fmt.Sprintf("-t nat -A PREROUTING -d %s -p %s --dport %d -j REDIRECT --to-port %d",
			eIP.String(), strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]), iPort.PublishedPort, iPort.TargetPort))
		rules = append(rules, rule)
		// Allow only incoming connections to exposed ports
		iRule := strings.Fields(fmt.Sprintf("-I INPUT -d %s -p %s --dport %d -m conntrack --ctstate NEW,ESTABLISHED -j ACCEPT",
			eIP.String(), strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]), iPort.TargetPort))
		rules = append(rules, iRule)
		// Allow only outgoing connections from exposed ports
		oRule := strings.Fields(fmt.Sprintf("-I OUTPUT -s %s -p %s --sport %d -m conntrack --ctstate ESTABLISHED -j ACCEPT",
			eIP.String(), strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]), iPort.TargetPort))
		rules = append(rules, oRule)
	}

	ns, err := netns.GetFromPath(path)
	if err != nil {
		return 4, fmt.Errorf("failed get network namespace %q: %v", path, err)
	}
	defer ns.Close()

	if err := netns.Set(ns); err != nil {
		return 5, fmt.Errorf("setting into container net ns %v failed, %v", os.Args[1], err)
	}

	for _, rule := range rules {
		if err := iptables.RawCombinedOutputNative(rule...); err != nil {
			return 6, fmt.Errorf("setting up rule failed, %v: %v", rule, err)
		}
	}

	if len(ingressPorts) == 0 {
		return 0, nil
	}

	// Ensure blocking rules for anything else in/to ingress network
	for _, rule := range [][]string{
		{"-d", eIP.String(), "-p", "udp", "-j", "DROP"},
		{"-d", eIP.String(), "-p", "tcp", "-j", "DROP"},
	} {
		if !iptables.ExistsNative(iptables.Filter, "INPUT", rule...) {
			if err := iptables.RawCombinedOutputNative(append([]string{"-A", "INPUT"}, rule...)...); err != nil {
				return 7, fmt.Errorf("setting up rule failed, %v: %v", rule, err)
			}
		}
		rule[0] = "-s"
		if !iptables.ExistsNative(iptables.Filter, "OUTPUT", rule...) {
			if err := iptables.RawCombinedOutputNative(append([]string{"-A", "OUTPUT"}, rule...)...); err != nil {
				return 8, fmt.Errorf("setting up rule failed, %v: %v", rule, err)
			}
		}
	}
	return 0, nil
}

func readPortsFromFile(fileName string) ([]*types.PortConfig, error) {
	buf, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var epRec types.EndpointRecord
	err = proto.Unmarshal(buf, &epRec)
	if err != nil {
		return nil, err
	}

	return epRec.IngressPorts, nil
}
