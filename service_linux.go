package libnetwork

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libnetwork/iptables"
	"github.com/docker/libnetwork/ipvs"
	"github.com/docker/libnetwork/ns"
	"github.com/docker/libnetwork/types"
	"github.com/gogo/protobuf/proto"
	"github.com/vishvananda/netlink/nl"
)

const servicerCommandName = "docker-network-servicer"

// Get all loadbalancers on this network that is currently discovered
// on this node.
func (n *network) connectedLoadbalancers() []*loadBalancer {
	c := n.getController()

	c.Lock()
	serviceBindings := make([]*service, 0, len(c.serviceBindings))
	for _, s := range c.serviceBindings {
		serviceBindings = append(serviceBindings, s)
	}
	c.Unlock()

	var lbs []*loadBalancer
	for _, s := range serviceBindings {
		s.Lock()
		if lb, ok := s.loadBalancers[n.ID()]; ok {
			lbs = append(lbs, lb)
		}
		s.Unlock()
	}

	return lbs
}

// Populate all loadbalancers on the network that the passed endpoint
// belongs to, into this sandbox.
func (sb *sandbox) populateLoadbalancers(ep *endpoint) {
	var gwIP net.IP

	// This is an interface less endpoint. Nothing to do.
	if ep.Iface() == nil {
		return
	}

	n := ep.getNetwork()
	eIP := ep.Iface().Address()

	if n.ingress {
		if err := addRedirectRules(sb.Key(), eIP, ep.ingressPorts); err != nil {
			logrus.Errorf("Failed to add redirect rules for ep %s: %v", ep.Name(), err)
		}
	}

	if sb.ingress {
		// For the ingress sandbox if this is not gateway
		// endpoint do nothing.
		if ep != sb.getGatewayEndpoint() {
			return
		}

		// This is the gateway endpoint. Now get the ingress
		// network and plumb the loadbalancers.
		gwIP = ep.Iface().Address().IP
		for _, ep := range sb.getConnectedEndpoints() {
			if !ep.endpointInGWNetwork() {
				n = ep.getNetwork()
				eIP = ep.Iface().Address()
			}
		}
	}

	for _, lb := range n.connectedLoadbalancers() {
		// Skip if vip is not valid.
		if len(lb.vip) == 0 {
			continue
		}

		lb.service.Lock()
		addService := true
		for _, ip := range lb.backEnds {
			sb.addLBBackend(ip, lb.vip, lb.fwMark, lb.service.ingressPorts,
				eIP, gwIP, addService, n.ingress)
			addService = false
		}
		lb.service.Unlock()
	}
}

// Add loadbalancer backend to all sandboxes which has a connection to
// this network. If needed add the service as well, as specified by
// the addService bool.
func (n *network) addLBBackend(ip, vip net.IP, fwMark uint32, ingressPorts []*types.PortConfig, addService bool) {
	n.WalkEndpoints(func(e Endpoint) bool {
		ep := e.(*endpoint)
		if sb, ok := ep.getSandbox(); ok {
			if !sb.isEndpointPopulated(ep) {
				return false
			}

			var gwIP net.IP
			if ep := sb.getGatewayEndpoint(); ep != nil {
				gwIP = ep.Iface().Address().IP
			}

			sb.addLBBackend(ip, vip, fwMark, ingressPorts, ep.Iface().Address(), gwIP, addService, n.ingress)
		}

		return false
	})
}

// Remove loadbalancer backend from all sandboxes which has a
// connection to this network. If needed remove the service entry as
// well, as specified by the rmService bool.
func (n *network) rmLBBackend(ip, vip net.IP, fwMark uint32, ingressPorts []*types.PortConfig, rmService bool) {
	n.WalkEndpoints(func(e Endpoint) bool {
		ep := e.(*endpoint)
		if sb, ok := ep.getSandbox(); ok {
			if !sb.isEndpointPopulated(ep) {
				return false
			}

			var gwIP net.IP
			if ep := sb.getGatewayEndpoint(); ep != nil {
				gwIP = ep.Iface().Address().IP
			}

			sb.rmLBBackend(ip, vip, fwMark, ingressPorts, ep.Iface().Address(), gwIP, rmService, n.ingress)
		}

		return false
	})
}

// Add loadbalancer backend into one connected sandbox.
func (sb *sandbox) addLBBackend(ip, vip net.IP, fwMark uint32, ingressPorts []*types.PortConfig, eIP *net.IPNet, gwIP net.IP, addService bool, isIngressNetwork bool) {
	if sb.osSbox == nil {
		return
	}

	if isIngressNetwork && !sb.ingress {
		return
	}

	i, err := ipvs.New(sb.Key())
	if err != nil {
		logrus.Errorf("Failed to create an ipvs handle for sbox %s: %v", sb.Key(), err)
		return
	}
	defer i.Close()

	s := &ipvs.Service{
		AddressFamily: nl.FAMILY_V4,
		FWMark:        fwMark,
		SchedName:     ipvs.RoundRobin,
	}

	if addService {
		var filteredPorts []*types.PortConfig
		if sb.ingress {
			filteredPorts = filterPortConfigs(ingressPorts, false)
			if err := programIngress(gwIP, filteredPorts, false); err != nil {
				logrus.Errorf("Failed to add ingress: %v", err)
				return
			}
		}

		logrus.Debugf("Creating service for vip %s fwMark %d ingressPorts %#v", vip, fwMark, ingressPorts)
		if err := invokeFWMarker(sb.Key(), vip, fwMark, ingressPorts, eIP, false); err != nil {
			logrus.Errorf("Failed to add firewall mark rule in sbox %s: %v", sb.Key(), err)
			return
		}

		if err := i.NewService(s); err != nil {
			logrus.Errorf("Failed to create a new service for vip %s fwmark %d: %v", vip, fwMark, err)
			return
		}
	}

	d := &ipvs.Destination{
		AddressFamily: nl.FAMILY_V4,
		Address:       ip,
		Weight:        1,
	}

	// Remove the sched name before using the service to add
	// destination.
	s.SchedName = ""
	if err := i.NewDestination(s, d); err != nil && err != syscall.EEXIST {
		logrus.Errorf("Failed to create real server %s for vip %s fwmark %d in sb %s: %v", ip, vip, fwMark, sb.containerID, err)
	}
}

// Remove loadbalancer backend from one connected sandbox.
func (sb *sandbox) rmLBBackend(ip, vip net.IP, fwMark uint32, ingressPorts []*types.PortConfig, eIP *net.IPNet, gwIP net.IP, rmService bool, isIngressNetwork bool) {
	if sb.osSbox == nil {
		return
	}

	if isIngressNetwork && !sb.ingress {
		return
	}

	i, err := ipvs.New(sb.Key())
	if err != nil {
		logrus.Errorf("Failed to create an ipvs handle for sbox %s: %v", sb.Key(), err)
		return
	}
	defer i.Close()

	s := &ipvs.Service{
		AddressFamily: nl.FAMILY_V4,
		FWMark:        fwMark,
	}

	d := &ipvs.Destination{
		AddressFamily: nl.FAMILY_V4,
		Address:       ip,
		Weight:        1,
	}

	if err := i.DelDestination(s, d); err != nil {
		logrus.Infof("Failed to delete real server %s for vip %s fwmark %d: %v", ip, vip, fwMark, err)
	}

	if rmService {
		s.SchedName = ipvs.RoundRobin
		if err := i.DelService(s); err != nil {
			logrus.Errorf("Failed to delete a new service for vip %s fwmark %d: %v", vip, fwMark, err)
		}

		var filteredPorts []*types.PortConfig
		if sb.ingress {
			filteredPorts = filterPortConfigs(ingressPorts, true)
			if err := programIngress(gwIP, filteredPorts, true); err != nil {
				logrus.Errorf("Failed to delete ingress: %v", err)
			}
		}

		if err := invokeFWMarker(sb.Key(), vip, fwMark, ingressPorts, eIP, true); err != nil {
			logrus.Errorf("Failed to add firewall mark rule in sbox %s: %v", sb.Key(), err)
		}
	}
}

const ingressChain = "DOCKER-INGRESS"

var (
	ingressOnce     sync.Once
	ingressProxyMu  sync.Mutex
	ingressProxyTbl = make(map[string]io.Closer)
	portConfigMu    sync.Mutex
	portConfigTbl   = make(map[types.PortConfig]int)
)

func filterPortConfigs(ingressPorts []*types.PortConfig, isDelete bool) []*types.PortConfig {
	portConfigMu.Lock()
	iPorts := make([]*types.PortConfig, 0, len(ingressPorts))
	for _, pc := range ingressPorts {
		if isDelete {
			if cnt, ok := portConfigTbl[*pc]; ok {
				// This is the last reference to this
				// port config. Delete the port config
				// and add it to filtered list to be
				// plumbed.
				if cnt == 1 {
					delete(portConfigTbl, *pc)
					iPorts = append(iPorts, pc)
					continue
				}

				portConfigTbl[*pc] = cnt - 1
			}

			continue
		}

		if cnt, ok := portConfigTbl[*pc]; ok {
			portConfigTbl[*pc] = cnt + 1
			continue
		}

		// We are adding it for the first time. Add it to the
		// filter list to be plumbed.
		portConfigTbl[*pc] = 1
		iPorts = append(iPorts, pc)
	}
	portConfigMu.Unlock()

	return iPorts
}

func programIngress(gwIP net.IP, ingressPorts []*types.PortConfig, isDelete bool) error {
	addDelOpt := "-I"
	if isDelete {
		addDelOpt = "-D"
	}

	chainExists := iptables.ExistChain(ingressChain, iptables.Nat)
	filterChainExists := iptables.ExistChain(ingressChain, iptables.Filter)

	ingressOnce.Do(func() {
		// Flush nat table and filter table ingress chain rules during init if it
		// exists. It might contain stale rules from previous life.
		if chainExists {
			if err := iptables.RawCombinedOutput("-t", "nat", "-F", ingressChain); err != nil {
				logrus.Errorf("Could not flush nat table ingress chain rules during init: %v", err)
			}
		}
		if filterChainExists {
			if err := iptables.RawCombinedOutput("-F", ingressChain); err != nil {
				logrus.Errorf("Could not flush filter table ingress chain rules during init: %v", err)
			}
		}
	})

	if !isDelete {
		if !chainExists {
			if err := iptables.RawCombinedOutput("-t", "nat", "-N", ingressChain); err != nil {
				return fmt.Errorf("failed to create ingress chain: %v", err)
			}
		}
		if !filterChainExists {
			if err := iptables.RawCombinedOutput("-N", ingressChain); err != nil {
				return fmt.Errorf("failed to create filter table ingress chain: %v", err)
			}
		}

		if !iptables.Exists(iptables.Nat, ingressChain, "-j", "RETURN") {
			if err := iptables.RawCombinedOutput("-t", "nat", "-A", ingressChain, "-j", "RETURN"); err != nil {
				return fmt.Errorf("failed to add return rule in nat table ingress chain: %v", err)
			}
		}

		if !iptables.Exists(iptables.Filter, ingressChain, "-j", "RETURN") {
			if err := iptables.RawCombinedOutput("-A", ingressChain, "-j", "RETURN"); err != nil {
				return fmt.Errorf("failed to add return rule to filter table ingress chain: %v", err)
			}
		}

		for _, chain := range []string{"OUTPUT", "PREROUTING"} {
			if !iptables.Exists(iptables.Nat, chain, "-m", "addrtype", "--dst-type", "LOCAL", "-j", ingressChain) {
				if err := iptables.RawCombinedOutput("-t", "nat", "-I", chain, "-m", "addrtype", "--dst-type", "LOCAL", "-j", ingressChain); err != nil {
					return fmt.Errorf("failed to add jump rule in %s to ingress chain: %v", chain, err)
				}
			}
		}

		if !iptables.Exists(iptables.Filter, "FORWARD", "-j", ingressChain) {
			if err := iptables.RawCombinedOutput("-I", "FORWARD", "-j", ingressChain); err != nil {
				return fmt.Errorf("failed to add jump rule to %s in filter table forward chain: %v", ingressChain, err)
			}
		}

		oifName, err := findOIFName(gwIP)
		if err != nil {
			return fmt.Errorf("failed to find gateway bridge interface name for %s: %v", gwIP, err)
		}

		path := filepath.Join("/proc/sys/net/ipv4/conf", oifName, "route_localnet")
		if err := ioutil.WriteFile(path, []byte{'1', '\n'}, 0644); err != nil {
			return fmt.Errorf("could not write to %s: %v", path, err)
		}

		ruleArgs := strings.Fields(fmt.Sprintf("-m addrtype --src-type LOCAL -o %s -j MASQUERADE", oifName))
		if !iptables.Exists(iptables.Nat, "POSTROUTING", ruleArgs...) {
			if err := iptables.RawCombinedOutput(append([]string{"-t", "nat", "-I", "POSTROUTING"}, ruleArgs...)...); err != nil {
				return fmt.Errorf("failed to add ingress localhost POSTROUTING rule for %s: %v", oifName, err)
			}
		}
	}

	for _, iPort := range ingressPorts {
		if iptables.ExistChain(ingressChain, iptables.Nat) {
			rule := strings.Fields(fmt.Sprintf("-t nat %s %s -p %s --dport %d -j DNAT --to-destination %s:%d",
				addDelOpt, ingressChain, strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]), iPort.PublishedPort, gwIP, iPort.PublishedPort))
			if err := iptables.RawCombinedOutput(rule...); err != nil {
				errStr := fmt.Sprintf("setting up rule failed, %v: %v", rule, err)
				if !isDelete {
					return fmt.Errorf("%s", errStr)
				}

				logrus.Infof("%s", errStr)
			}
		}

		// Filter table rules to allow a published service to be accessible in the local node from..
		// 1) service tasks attached to other networks
		// 2) unmanaged containers on bridge networks
		rule := strings.Fields(fmt.Sprintf("%s %s -m state -p %s --sport %d --state ESTABLISHED,RELATED -j ACCEPT",
			addDelOpt, ingressChain, strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]), iPort.PublishedPort))
		if err := iptables.RawCombinedOutput(rule...); err != nil {
			errStr := fmt.Sprintf("setting up rule failed, %v: %v", rule, err)
			if !isDelete {
				return fmt.Errorf("%s", errStr)
			}
			logrus.Warnf("%s", errStr)
		}

		rule = strings.Fields(fmt.Sprintf("%s %s -p %s --dport %d -j ACCEPT",
			addDelOpt, ingressChain, strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]), iPort.PublishedPort))
		if err := iptables.RawCombinedOutput(rule...); err != nil {
			errStr := fmt.Sprintf("setting up rule failed, %v: %v", rule, err)
			if !isDelete {
				return fmt.Errorf("%s", errStr)
			}

			logrus.Warnf("%s", errStr)
		}

		if err := plumbProxy(iPort, isDelete); err != nil {
			logrus.Warnf("failed to create proxy for port %d: %v", iPort.PublishedPort, err)
		}
	}

	return nil
}

// In the filter table FORWARD chain first rule should be to jump to INGRESS-CHAIN
// This chain has the rules to allow access to the published ports for swarm tasks
// from local bridge networks and docker_gwbridge (ie:taks on other swarm netwroks)
func arrangeIngressFilterRule() {
	if iptables.ExistChain(ingressChain, iptables.Filter) {
		if iptables.Exists(iptables.Filter, "FORWARD", "-j", ingressChain) {
			if err := iptables.RawCombinedOutput("-D", "FORWARD", "-j", ingressChain); err != nil {
				logrus.Warnf("failed to delete jump rule to ingressChain in filter table: %v", err)
			}
		}
		if err := iptables.RawCombinedOutput("-I", "FORWARD", "-j", ingressChain); err != nil {
			logrus.Warnf("failed to add jump rule to ingressChain in filter table: %v", err)
		}
	}
}

func findOIFName(ip net.IP) (string, error) {
	nlh := ns.NlHandle()

	routes, err := nlh.RouteGet(ip)
	if err != nil {
		return "", err
	}

	if len(routes) == 0 {
		return "", fmt.Errorf("no route to %s", ip)
	}

	// Pick the first route(typically there is only one route). We
	// don't support multipath.
	link, err := nlh.LinkByIndex(routes[0].LinkIndex)
	if err != nil {
		return "", err
	}

	return link.Attrs().Name, nil
}

func plumbProxy(iPort *types.PortConfig, isDelete bool) error {
	var (
		err error
		l   io.Closer
	)

	portSpec := fmt.Sprintf("%d/%s", iPort.PublishedPort, strings.ToLower(types.PortConfig_Protocol_name[int32(iPort.Protocol)]))
	if isDelete {
		ingressProxyMu.Lock()
		if listener, ok := ingressProxyTbl[portSpec]; ok {
			if listener != nil {
				listener.Close()
			}
		}
		ingressProxyMu.Unlock()

		return nil
	}

	switch iPort.Protocol {
	case types.ProtocolTCP:
		l, err = net.ListenTCP("tcp", &net.TCPAddr{Port: int(iPort.PublishedPort)})
	case types.ProtocolUDP:
		l, err = net.ListenUDP("udp", &net.UDPAddr{Port: int(iPort.PublishedPort)})
	}

	if err != nil {
		return err
	}

	ingressProxyMu.Lock()
	ingressProxyTbl[portSpec] = l
	ingressProxyMu.Unlock()

	return nil
}

func writePortsToFile(ports []*types.PortConfig) (string, error) {
	f, err := ioutil.TempFile("", "port_configs")
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf, err := proto.Marshal(&types.EndpointRecord{
		IngressPorts: ports,
	})

	n, err := f.Write(buf)
	if err != nil {
		return "", err
	}

	if n < len(buf) {
		return "", io.ErrShortWrite
	}

	return f.Name(), nil
}

// Invoke fwmarker binary to mark vip destined packets with
// the passed firewall mark.
func invokeFWMarker(path string, vip net.IP, fwMark uint32, ingressPorts []*types.PortConfig, eIP *net.IPNet, isDelete bool) error {
	var ingressPortsFile string

	if len(ingressPorts) != 0 {
		var err error
		ingressPortsFile, err = writePortsToFile(ingressPorts)
		if err != nil {
			return err
		}

		defer os.Remove(ingressPortsFile)
	}

	execPath, err := exec.LookPath(servicerCommandName)
	if err != nil {
		return fmt.Errorf("failed to find %s binary: %v", servicerCommandName, err)
	}

	args := []string{
		execPath,
		"marker",
		"-path", path,
		"-vip", vip.String(),
		"-eip", eIP.String(),
		"-ports-file", ingressPortsFile,
		"-mark", fmt.Sprintf("%d", fwMark),
	}

	if isDelete {
		args = append(args, "-del")
	}

	cmd := &exec.Cmd{
		Path:   execPath,
		Args:   args,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("marker failed: %v", err)
	}

	return nil
}

func addRedirectRules(path string, eIP *net.IPNet, ingressPorts []*types.PortConfig) error {
	var ingressPortsFile string

	if len(ingressPorts) != 0 {
		var err error
		ingressPortsFile, err = writePortsToFile(ingressPorts)
		if err != nil {
			return err
		}
		defer os.Remove(ingressPortsFile)
	}

	execPath, err := exec.LookPath(servicerCommandName)
	if err != nil {
		return fmt.Errorf("failed to find %s binary: %v", servicerCommandName, err)
	}

	args := []string{
		execPath,
		"redirecter",
		"-path", path,
		"-eip", eIP.String(),
		"-ports-file", ingressPortsFile,
	}

	cmd := &exec.Cmd{
		Path:   execPath,
		Args:   args,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("redirecter failed: %v", err)
	}

	return nil
}
