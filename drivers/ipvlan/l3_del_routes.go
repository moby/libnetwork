package ipvlan

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"net"
)

// Cleanup links with netlink syscalls with a scope of:
// RT_SCOPE_LINK = 0xfd (253)
// RT_SCOPE_UNIVERSE = 0x0 (0)
func cleanExistingRoutes(ifaceStr string) error {
	iface, err := netlink.LinkByName(ifaceStr)
	ipvlanParentIface, err := netlink.LinkByName(ifaceStr)
	if err != nil {
		log.Errorf("Error occoured finding the linux link [ %s ] from netlink: %s", ipvlanParentIface.Attrs().Name, err)
		return err
	}
	routes, err := netlink.RouteList(iface, netlink.FAMILY_V4)
	if err != nil {
		log.Errorf("Unable to retreive netlink routes: %s", err)
		return err
	}
	ifaceIP, err := getIfaceIP(ifaceStr)
	if err != nil {
		log.Errorf("Unable to retreive a usable IP via ethernet interface: %s", ifaceStr)
		return err
	}
	for _, route := range routes {
		if route.Dst == nil {
			log.Debugf("Ignoring route [ %v ] Dst is nil", route)
			continue
		}
		if netOverlaps(ifaceIP, route.Dst) == true {
			log.Debugf("Ignoring route [ %v ] as it is associated to the [ %s ] interface", ifaceIP, ifaceStr)
		} else if route.Scope == 0x0 || route.Scope == 0xfd {
			// Remove link and universal routes from the docker host ipvlan interface
			log.Debugf("Cleaning static route cache for the destination: [ %s ]", route.Dst)
			err := delRoute(route, ipvlanParentIface)
			if err != nil {
				log.Errorf("Error deleting static route cache for Destination: [ %s ] and Nexthop [ %s ] Error: %s", route.Dst, route.Gw, err)
			}
		}
	}
	return nil
}

func verifyRoute(bgpRoute *net.IPNet) {
	networks, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return
	}
	for _, network := range networks {
		if network.Dst != nil && netOverlaps(bgpRoute, network.Dst) {
			log.Errorf("The network [ %v ] learned via BGP conflicts with an existing route on this host [ %v ]", bgpRoute, network.Dst)
			return
		}
	}
	return
}

func netOverlaps(netX *net.IPNet, netY *net.IPNet) bool {
	if firstIP, _ := networkRange(netX); netY.Contains(firstIP) {
		return true
	}
	if firstIP, _ := networkRange(netY); netX.Contains(firstIP) {
		return true
	}
	return false
}

func networkRange(network *net.IPNet) (net.IP, net.IP) {
	var (
		netIP   = network.IP.To4()
		firstIP = netIP.Mask(network.Mask)
		lastIP  = net.IPv4(0, 0, 0, 0).To4()
	)

	for i := 0; i < len(lastIP); i++ {
		lastIP[i] = netIP[i] | ^network.Mask[i]
	}
	return firstIP, lastIP
}

func getIfaceIP(name string) (*net.IPNet, error) {
	iface, err := netlink.LinkByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := netlink.AddrList(iface, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("Interface %v has no IP addresses", name)
	}
	if len(addrs) > 1 {
		log.Debugf("Interface %v has more than 1 IPv4 address. Default is %v\n", name, addrs[0].IP)
	}
	return addrs[0].IPNet, nil
}

// delRoute deletes any netlink route
func delRoute(route netlink.Route, iface netlink.Link) error {
	return netlink.RouteDel(&netlink.Route{
		Scope:     route.Scope,
		LinkIndex: iface.Attrs().Index,
		Dst:       route.Dst,
		Gw:        route.Gw,
	})
}

// delRemoteRoute deletes a host-scoped route to a device.
func delRemoteRoute(neighborNetwork *net.IPNet, nextHop net.IP, iface netlink.Link) error {
	return netlink.RouteDel(&netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		LinkIndex: iface.Attrs().Index,
		Dst:       neighborNetwork,
		Gw:        nextHop,
	})
}
