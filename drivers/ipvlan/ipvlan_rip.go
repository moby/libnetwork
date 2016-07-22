package ipvlan

import (
	"bytes"
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

const (
	// Default RIP settings
	defaultRipUpdateTimer = 30  // in seconds
	defaultRipUpdateDelay = 0.1 // in seconds
	defaultRipGCTimer     = 180 // in seconds
	defaultRipMetric      = 1
	defaultRipTag         = 0
)

// RipManagers is a map of RipManager instances. Map key is the network ID.
var RipManagers map[string]RipManager

// RipManager object. Each ipvlan L3 network has its own instance of the
// RipManager.
type RipManager struct {
	NetworkID string
	ParentIf  string
	// Timers
	UpdateTimer time.Duration
	UpdateDelay time.Duration
	GCTimer     time.Duration
	// RIP fields
	Metric   uint8
	RouteTag uint16
	// IPv4/IPv6 specific settings
	IPv4 RipManagerSettings
	IPv6 RipManagerSettings
}

// RipManagerSettings for each IP version family
type RipManagerSettings struct {
	Enabled bool
	// A map of routes being advertised on the parent interface
	RouteTable map[string]RouteTableEntry
	// Prevents reading from the routeTable when routeTable is being altered
	RouteTableMutex *sync.RWMutex
	// Triggers a RIP update
	TriggeredUpdate chan bool
}

// RouteTableEntry holds a RIP route
type RouteTableEntry struct {
	Updated time.Time
	Route   net.IPNet
	Deleted bool
}

// NewRipManager creates a new RipManager and adds it to the RipManagers map
func NewRipManager(config *configuration) {
	if RipManagers == nil {
		RipManagers = make(map[string]RipManager)
	}
	RipManager := RipManager{}
	RipManager.NetworkID = config.ID
	RipManager.ParentIf = config.Parent

	if config.RipUpdateTimer != 0 {
		RipManager.UpdateTimer = config.RipUpdateTimer
	} else {
		RipManager.UpdateTimer = time.Duration(int(defaultRipUpdateTimer*1000)) * time.Millisecond
	}
	if config.RipUpdateDelay != 0 {
		RipManager.UpdateDelay = config.RipUpdateDelay
	} else {
		RipManager.UpdateDelay = time.Duration(int(defaultRipUpdateDelay*1000)) * time.Millisecond
	}
	if config.RipGCTimer != 0 {
		RipManager.GCTimer = config.RipGCTimer
	} else {
		RipManager.GCTimer = time.Duration(int(defaultRipGCTimer*1000)) * time.Millisecond
	}
	if config.RipMetric != 0 {
		RipManager.Metric = config.RipMetric
	} else {
		RipManager.Metric = defaultRipMetric
	}
	if config.RipTag != 0 {
		RipManager.RouteTag = config.RipTag
	} else {
		RipManager.RouteTag = defaultRipTag
	}

	if config.RipIPv4 {
		RipManager.IPv4.Enabled = true
		RipManager.IPv4.RouteTableMutex = &sync.RWMutex{}
		RipManager.IPv4.TriggeredUpdate = make(chan bool)
		if len(config.Ipv4Subnets) > 0 {
			RipManager.IPv4.RouteTableMutex.Lock()
			RipManager.IPv4.RouteTable = make(map[string]RouteTableEntry)
			if config.RipAdvertiseNetworks {
				for _, ipv4Subnet := range config.Ipv4Subnets {
					ip, ipNet, err := net.ParseCIDR(ipv4Subnet.SubnetIP)
					if ip.To4() == nil {
						//logrus.Errorf("Static route configuration: %s is not an IPv4 address", ip.String())
					} else if err != nil {
						//logrus.Errorf("Static route configuration: %s", err.Error())
					} else {
						RipManager.addIpv4Route(*ipNet, false)
					}
				}
			}
			RipManager.IPv4.RouteTableMutex.Unlock()
		}
		go RipManager.ipv4ripRouter()
	}

	if config.RipIPv6 {
		RipManager.IPv6.Enabled = true
		RipManager.IPv6.RouteTableMutex = &sync.RWMutex{}
		RipManager.IPv6.TriggeredUpdate = make(chan bool)
		if len(config.Ipv6Subnets) > 0 {
			RipManager.IPv6.RouteTableMutex.Lock()
			RipManager.IPv6.RouteTable = make(map[string]RouteTableEntry)
			if config.RipAdvertiseNetworks {
				for _, ipv6Subnet := range config.Ipv6Subnets {
					ip, ipNet, err := net.ParseCIDR(ipv6Subnet.SubnetIP)
					if ip.To16() == nil {
						//logrus.Errorf("Static route configuration: %s is not an IPv4 address", ip.String())
					} else if err != nil {
						//logrus.Errorf("Static route configuration: %s", err.Error())
					} else {
						RipManager.addIpv6Route(*ipNet, false)
					}
				}
			}
			RipManager.IPv6.RouteTableMutex.Unlock()
		}
		go RipManager.ipv6ripRouter()
	}

	RipManagers[config.ID] = RipManager
}

// AddIpv4Endpoint starts advertising an IPv4 host
// Converts endpoint data to a /32 route and calls addIpv4Route()
func (RipManager RipManager) AddIpv4Endpoint(epAddr net.IPNet) error {
	// epAddr holds endpoint's IP address with network's subnet mask
	// To advertise the endpoint route, we will use endpoint's IP address
	// with /32 subnet mask
	route := epAddr
	route.Mask = net.CIDRMask(32, 32)
	RipManager.IPv4.RouteTableMutex.Lock()
	err := RipManager.addIpv4Route(route, true)
	RipManager.IPv4.RouteTableMutex.Unlock()
	return err
}

// AddIpv6Endpoint starts advertising an IPv6 host
// Converts endpoint data to a /128 route and calls addIpv4Route()
func (RipManager RipManager) AddIpv6Endpoint(epAddrv6 net.IPNet) error {
	// epAddrv6 holds endpoint's IPv6 address with network's subnet mask
	// To advertise the endpoint route, we will use endpoint's IPv6 address
	// with /128 subnet mask
	route := epAddrv6
	route.Mask = net.CIDRMask(128, 128)
	RipManager.IPv6.RouteTableMutex.Lock()
	err := RipManager.addIpv6Route(route, true)
	RipManager.IPv6.RouteTableMutex.Unlock()
	return err
}

// DeleteIpv4Endpoint stops advertising an IPv4 host
// Converts endpoint data to a /32 route and calls delIpv4Route()
func (RipManager RipManager) DeleteIpv4Endpoint(epAddr net.IPNet) error {
	// epAddr holds endpoint's IP address with network's subnet mask
	// To advertise the endpoint route, we will use endpoint's IP address
	// with /32 subnet mask
	route := epAddr
	route.Mask = net.CIDRMask(32, 32)
	RipManager.IPv4.RouteTableMutex.Lock()
	err := RipManager.deleteIpv4Route(route, true)
	RipManager.IPv4.RouteTableMutex.Unlock()
	return err
}

// DeleteIpv6Endpoint stops advertising an IPv6 host
// Converts endpoint data to a /128 route and calls delIpv6Route()
func (RipManager RipManager) DeleteIpv6Endpoint(epAddrv6 net.IPNet) error {
	// epAddrv6 holds endpoint's IPv6 address with network's subnet mask
	// To advertise the endpoint route, we will use endpoint's IPv6 address
	// with /128 subnet mask
	route := epAddrv6
	route.Mask = net.CIDRMask(128, 128)
	RipManager.IPv6.RouteTableMutex.Lock()
	err := RipManager.deleteIpv6Route(route, true)
	RipManager.IPv6.RouteTableMutex.Unlock()
	return err
}

// addIpv4Route adds an IPv4 route to the RipManager.IPv4.RouteTable and
// optionally triggers an update.
// RipManager.IPv4.RouteTableMutex must be Lock()ed before this function call
// and Unlock()ed after.
func (RipManager RipManager) addIpv4Route(route net.IPNet, triggerUpdate bool) error {
	routeTableEntry := new(RouteTableEntry)
	routeTableEntry.Updated = time.Now()
	routeTableEntry.Route = route
	routeTableEntry.Deleted = false
	RipManager.IPv4.RouteTable[route.String()] = *routeTableEntry
	logrus.Debugf("Ipvlan L3 RIP: Adding %s to %s IPv4 route table", route.String(), RipManager.ParentIf)
	if triggerUpdate == true {
		select {
		case RipManager.IPv4.TriggeredUpdate <- true:
			//
		case <-time.After(time.Duration(5) * time.Second):
			// To prevent unforseen channel block-locks, continue if channel is blocked for more than 5 seconds
			logrus.Debugf("Ipvlan L3 RIP: Tried to add an IPv4 route, but ripRouter on %s is not running", RipManager.ParentIf)
		}
	}
	return nil
}

// addIpv6Route adds an IPv6 route to the  RipManager.IPv6.RouteTable and
// optionally triggers an update.
// RipManager.IPv6.RouteTableMutex must be Lock()ed for this function call
// and Unlock()ed after.
func (RipManager RipManager) addIpv6Route(route net.IPNet, triggerUpdate bool) error {
	routeTableEntry := new(RouteTableEntry)
	routeTableEntry.Updated = time.Now()
	routeTableEntry.Route = route
	routeTableEntry.Deleted = false
	RipManager.IPv6.RouteTable[route.String()] = *routeTableEntry
	logrus.Debugf("Ipvlan L3 RIP: Adding %s to %s IPv6 route table", route.String(), RipManager.ParentIf)
	if triggerUpdate == true {
		select {
		case RipManager.IPv6.TriggeredUpdate <- true:
			//
		case <-time.After(time.Duration(5) * time.Second):
			// To prevent unforseen channel block-locks, continue if channel is blocked for more than 5 seconds
			logrus.Debugf("Ipvlan L3 RIP: Tried to add an IPv6 route, but ripRouter on %s is not running", RipManager.ParentIf)
		}
	}
	return nil
}

// deleteIpv4Route marks an IPv4 route as Deleted in the
// RipManager.IPv4.RouteTable and optionally triggers an update. Runs
// gcIpv4Routes to remove the route from RipManager.IPv4.RouteTable with a
// GCTimer delay. Deleted routes are advertised with metric 16 untill removed by
// gcIpv4Routes.
// RipManager.IPv4.RouteTableMutex must be Lock()ed for this function call
// and Unlock()ed after
func (RipManager RipManager) deleteIpv4Route(route net.IPNet, triggerUpdate bool) error {
	routeTableEntry := new(RouteTableEntry)
	routeTableEntry.Updated = time.Now()
	routeTableEntry.Route = route
	routeTableEntry.Deleted = true
	RipManager.IPv4.RouteTable[route.String()] = *routeTableEntry
	go RipManager.gcIpv4Routes()
	logrus.Debugf("Ipvlan L3 RIP: Deleting %s in %s IPv4 route table", route.String(), RipManager.ParentIf)
	if triggerUpdate == true {
		select {
		case RipManager.IPv4.TriggeredUpdate <- true:
			//
		case <-time.After(time.Duration(5) * time.Second):
			// To prevent unforseen channel block-locks, continue if channel is blocked for more than 5 seconds
			logrus.Debugf("Ipvlan L3 RIP: Tried to delete an IPv4 route, but ripRouter on %s is not running", RipManager.ParentIf)
		}
	}
	return nil
}

// deleteIpv6Route marks an IPv6 route as Deleted in the
// RipManager.IPv6.RouteTable and optionally triggers an update. Runs
// gcIpv6Routes to remove the route from RipManager.IPv6.RouteTable with a
// GCTimer delay. Deleted routes are advertised with metric 16 untill removed by
// gcIpv6Routes.
// RipManager.IPv6.RouteTableMutex must be Lock()ed for this function call
// and Unlock()ed after.
func (RipManager RipManager) deleteIpv6Route(route net.IPNet, triggerUpdate bool) error {
	routeTableEntry := new(RouteTableEntry)
	routeTableEntry.Updated = time.Now()
	routeTableEntry.Route = route
	routeTableEntry.Deleted = true
	RipManager.IPv6.RouteTable[route.String()] = *routeTableEntry
	go RipManager.gcIpv6Routes()
	logrus.Debugf("Ipvlan L3 RIP: Deleting %s in %s IPv6 route table", route.String(), RipManager.ParentIf)
	if triggerUpdate == true {
		select {
		case RipManager.IPv6.TriggeredUpdate <- true:
			//
		case <-time.After(time.Duration(5) * time.Second):
			// To prevent unforseen channel block-locks, continue if channel is blocked for more than 5 seconds
			logrus.Debugf("Ipvlan L3 RIP: Tried to delete an IPv6 route, but ripRouter on %s is not running", RipManager.ParentIf)
		}
	}
	return nil
}

// gcIpv4Routes is ran by deleteIpv4Route as a goroutine. Sleeps for GCTimer,
// than removes the deleted route from the RipManager.IPv4.RouteTable
func (RipManager RipManager) gcIpv4Routes() error {
	time.Sleep(RipManager.GCTimer)
	if RipManager.IPv4.RouteTable != nil {
		RipManager.IPv4.RouteTableMutex.Lock()
		for route, routeTableEntry := range RipManager.IPv4.RouteTable {
			if routeTableEntry.Deleted == true {
				if time.Since(routeTableEntry.Updated) > RipManager.GCTimer {
					delete(RipManager.IPv4.RouteTable, route)
					logrus.Debugf("Ipvlan L3 RIP: Removing %s from %s IPv4 route table", route, RipManager.ParentIf)
				}
			}
		}
		RipManager.IPv4.RouteTableMutex.Unlock()
	}
	return nil
}

// gcIpv6Routes is ran by deleteIpv6Route as a goroutine. Sleeps for GCTimer,
// than removes the deleted route from RipManager.IPv6.RouteTable
func (RipManager RipManager) gcIpv6Routes() error {
	time.Sleep(RipManager.GCTimer)
	if RipManager.IPv6.RouteTable != nil {
		RipManager.IPv6.RouteTableMutex.Lock()
		for route, routeTableEntry := range RipManager.IPv6.RouteTable {
			if routeTableEntry.Deleted == true {
				if time.Since(routeTableEntry.Updated) > RipManager.GCTimer {
					delete(RipManager.IPv6.RouteTable, route)
					logrus.Debugf("Ipvlan L3 RIP: Removing %s from %s IPv6 route table", route, RipManager.ParentIf)
				}
			}
		}
		RipManager.IPv6.RouteTableMutex.Unlock()
	}
	return nil
}

// ipv4ripRouter runs in Goroutine
// Advertises routes using RIPv2 every updateTimer or whenever there is a triggeredUpdate.
// Routes are advertised using the Unsolicited routing update message, multicasted
// from each of the parentIP addreses from UDP 520 to UDP 520.
func (RipManager RipManager) ipv4ripRouter() {

	logrus.Debugf("Ipvlan L3 RIP: Starting IPv4 RIP router on %s (update: %s, update delay: %s, gc: %s)", RipManager.ParentIf, RipManager.UpdateTimer.String(), RipManager.UpdateDelay.String(), RipManager.GCTimer.String())

	lastUpdate := time.Now().Add(-time.Hour)

	for {
		// All routeTable routes are advertised with regular updates every updateTimer
		// (defaults to 30s). Whenever there is a change in routeTable, a triggered
		// update is sent immediately, or, if an update has been just sent, after
		// a triggeredUpdateDelay
		nextUpdate := lastUpdate.Add(RipManager.UpdateTimer)
		delay := -time.Since(nextUpdate)
		select {
		case _, channelOpen := <-RipManager.IPv4.TriggeredUpdate:
			if !channelOpen {
				return
			} else if lastUpdate.Add(RipManager.UpdateDelay).After(time.Now()) {
				sleepDelay := RipManager.UpdateDelay - time.Since(lastUpdate)
				logrus.Debugf("Ipvlan L3 RIP: IPv4 update on %s throttled, sleeping %f seconds", RipManager.ParentIf, sleepDelay.Seconds())
				time.Sleep(sleepDelay)
			}
			logrus.Debugf("Ipvlan L3 RIP: Triggered IPv4 update on %s", RipManager.ParentIf)
		case <-time.After(delay):
			logrus.Debugf("Ipvlan L3 RIP: Regular IPv4 update on %s", RipManager.ParentIf)
		}
		RipManager.IPv4.RouteTableMutex.RLock()
		RipManager.sendIpv4RipMessage()
		RipManager.IPv4.RouteTableMutex.RUnlock()
		lastUpdate = time.Now()
	}

}

// ipv6ripRouter runs in Goroutine
// Advertises routes using RIPng every updateTimer or whenever there is a triggeredUpdate.
// Routes are advertised using the Unsolicited routing update message, multicasted
// from each of the parentIP link-local addreses from UDP 521 to UDP 521.
func (RipManager RipManager) ipv6ripRouter() {

	logrus.Debugf("Ipvlan L3 RIP: Starting IPv6 RIP router on %s (update: %s, update delay: %s, gc: %s)", RipManager.ParentIf, RipManager.UpdateTimer.String(), RipManager.UpdateDelay.String(), RipManager.GCTimer.String())

	lastUpdate := time.Now().Add(-time.Hour)

	for {
		// All routeTable routes are advertised with regular updates every updateTimer
		// (defaults to 30s). Whenever there is a change in routeTable, a triggered
		// update is sent immediately, or, if an update has been just sent, after
		// a triggeredUpdateDelay
		nextUpdate := lastUpdate.Add(RipManager.UpdateTimer)
		delay := -time.Since(nextUpdate)
		select {
		case _, channelOpen := <-RipManager.IPv6.TriggeredUpdate:
			if !channelOpen {
				return
			} else if lastUpdate.Add(RipManager.UpdateDelay).After(time.Now()) {
				sleepDelay := RipManager.UpdateDelay - time.Since(lastUpdate)
				logrus.Debugf("Ipvlan L3 RIP: IPv6 update on %s throttled, sleeping %f seconds", RipManager.ParentIf, sleepDelay.Seconds())
				time.Sleep(sleepDelay)
			}
			logrus.Debugf("Ipvlan L3 RIP: Triggered IPv6 update on %s", RipManager.ParentIf)
		case <-time.After(delay):
			logrus.Debugf("Ipvlan L3 RIP: Regular IPv6 update on %s", RipManager.ParentIf)
		}
		RipManager.IPv6.RouteTableMutex.RLock()
		RipManager.sendIpv6RipMessage()
		RipManager.IPv6.RouteTableMutex.RUnlock()
		lastUpdate = time.Now()
	}

}

// sendIpv4RipMessage sends out the RIPv2 Unsolicited routing update message,
// multicasted from each of the parentIP addreses.
func (RipManager RipManager) sendIpv4RipMessage() {

	// RIPv2 message structures, as per RFC2453, 4. Protocol Extensions
	type RIPv2Header struct {
		Command uint8
		Version uint8
		Unused  uint16
	}
	type RIPv2Route struct {
		AddressFamilyIdentifier uint16
		RouteTag                uint16
		IPAddress               uint32
		SubnetMask              uint32
		NextHop                 uint32
		Metric                  uint32
	}

	// Ipvlan L3 parent interface may have multiple IP addresses, advertise on all
	var parentIPs []net.IP

	if len(RipManager.IPv4.RouteTable) == 0 {
		logrus.Debugf("Ipvlan L3 RIP: Advertising no IPv4 routes from %s", RipManager.ParentIf)
		return
	}

	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if strings.ToLower(iface.Name) == strings.ToLower(RipManager.ParentIf) {
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					//
				} else if ip.To4() == nil {
					// This is not an IPv4 address
				} else {
					parentIPs = append(parentIPs, ip.To4())
				}
			}
			continue
		}
	}

	for _, parentIP := range parentIPs {
		localAddr, err := net.ResolveUDPAddr("udp4", parentIP.String()+":520")
		if err != nil {
			logrus.Debugf("Ipvlan L3 RIP: Resolution error %v", err)
			return
		}
		remoteEP, _ := net.ResolveUDPAddr("udp4", "224.0.0.9:520")
		conn, err := net.DialUDP("udp4", localAddr, remoteEP)
		if err != nil {
			logrus.Debugf("Ipvlan L3 RIP: Connection error %v", err)
			return
		}

		ripHeader := new(RIPv2Header)
		ripHeader.Command = 2
		ripHeader.Version = 2
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, ripHeader)

		routeStr := ""
		needsToBeWritten := false
		n := 0
		for _, routeTableEntry := range RipManager.IPv4.RouteTable {
			route := new(RIPv2Route)
			route.AddressFamilyIdentifier = 2
			route.IPAddress = binary.BigEndian.Uint32(routeTableEntry.Route.IP)
			route.SubnetMask = binary.BigEndian.Uint32(routeTableEntry.Route.Mask)
			if routeTableEntry.Deleted == true {
				// Deleted routes younger than gcTimer are advertised with:
				route.Metric = 16
			} else {
				route.Metric = uint32(RipManager.Metric)
			}
			route.RouteTag = RipManager.RouteTag
			// NextHop is left at 0.0.0.0. Receiving RIP router will use sender's
			// IP address as the NextHop.
			binary.Write(&buf, binary.BigEndian, route)
			maskOnes, _ := routeTableEntry.Route.Mask.Size()
			routeStr = routeStr + "" + routeTableEntry.Route.IP.String() + "/" + strconv.FormatUint(uint64(maskOnes), 10) + " (metric " + strconv.FormatUint(uint64(route.Metric), 10) + "), "

			// RFC2453 prescribes no more than 25 RTEs per UDP datagram
			// Send out an UDP datagram every 25 RTEs
			if n%25 == 24 {
				conn.Write(buf.Bytes())
				ripHeader = new(RIPv2Header)
				ripHeader.Command = 2
				ripHeader.Version = 2
				buf.Reset()
				binary.Write(&buf, binary.BigEndian, ripHeader)
				needsToBeWritten = false
			} else {
				needsToBeWritten = true
			}
			n++
		}

		if needsToBeWritten {
			conn.Write(buf.Bytes())
		}
		conn.Close()

		logrus.Debugf("Ipvlan L3 RIP: Advertising IPv4 routes from %s (%s): %s", parentIP, RipManager.ParentIf, routeStr[0:len(routeStr)-2])
	}
}

// sendIpv6RipMessage sends out the RIPng Unsolicited routing update message,
// multicasted from each of the parentIP link-local addreses.
func (RipManager RipManager) sendIpv6RipMessage() {

	// RIPng message structures, as per RFC2080
	type RIPngHeader struct {
		Command uint8
		Version uint8
		Unused  uint16
	}
	type RIPngRoute struct {
		IPAddress [16]byte
		RouteTag  uint16
		PrefixLen uint8
		Metric    uint8
	}

	var parentIface net.Interface
	// Ipvlan L3 parent interface may have multiple IP addresses, advertise on all
	// link-local addresses
	var parentIPs []net.IP

	if len(RipManager.IPv6.RouteTable) == 0 {
		logrus.Debugf("Ipvlan L3 RIP: Advertising no IPv6 routes from %s", RipManager.ParentIf)
		return
	}

	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if strings.ToLower(iface.Name) == strings.ToLower(RipManager.ParentIf) {
			parentIface = iface
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				addrStr := addr.String()
				ip, _, err := net.ParseCIDR(addrStr)
				if err != nil {
					//
				} else if ip.To16() == nil {
					// This is not an IPv6 address
				} else if addrStr[0:4] != "fe80" {
					// only advertise on link-local (per RFC2080)
				} else if ip.To4() == nil {
					parentIPs = append(parentIPs, ip.To16())
				}
			}
			continue
		}
	}
	for _, parentIP := range parentIPs {
		localAddr, err := net.ResolveUDPAddr("udp6", "["+parentIP.String()+"%"+RipManager.ParentIf+"]:521")
		if err != nil {
			logrus.Debugf("Ipvlan L3 RIP: Resolution error %v", err)
			return
		}
		remoteEP, err := net.ResolveUDPAddr("udp6", "[ff02::9]:521")
		if err != nil {
			logrus.Debugf("Ipvlan L3 RIP: Resolution error %v", err)
			return
		}
		conn, err := net.DialUDP("udp6", localAddr, remoteEP)
		if err != nil {
			logrus.Debugf("Ipvlan L3 RIP: Connection error %v", err)
			return
		}
		ripHeader := new(RIPngHeader)
		ripHeader.Command = 2
		ripHeader.Version = 1

		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, ripHeader)

		// Next hop RTE is not used, receiving router will use RIPng message's source
		// address as the next hop
		routeStr := ""
		needsToBeWritten := false
		for _, routeTableEntry := range RipManager.IPv6.RouteTable {
			route := new(RIPngRoute)
			copy(route.IPAddress[:], routeTableEntry.Route.IP)
			prefixLen, _ := routeTableEntry.Route.Mask.Size()
			route.PrefixLen = uint8(prefixLen)
			if routeTableEntry.Deleted == true {
				// Deleted routes younger than gcTimer are advertised with:
				route.Metric = 16
			} else {
				route.Metric = RipManager.Metric
			}
			route.RouteTag = RipManager.RouteTag
			binary.Write(&buf, binary.BigEndian, route)
			routeStr = routeStr + "" + routeTableEntry.Route.IP.String() + "/" + strconv.FormatUint(uint64(route.PrefixLen), 10) + " (metric " + strconv.FormatUint(uint64(route.Metric), 10) + "), "

			// RFC2080 prescribes UDP datagram should not exceed MTU - IPv6 header
			// Substract 20 for IPv6 header, and 20 for the last RTE included
			if buf.Len() > parentIface.MTU-40 {
				conn.Write(buf.Bytes())
				ripHeader = new(RIPngHeader)
				ripHeader.Command = 2
				ripHeader.Version = 2
				buf.Reset()
				binary.Write(&buf, binary.BigEndian, ripHeader)
				needsToBeWritten = false
			} else {
				needsToBeWritten = true
			}

		}

		if needsToBeWritten {
			conn.Write(buf.Bytes())
		}

		conn.Close()

		logrus.Debugf("Ipvlan L3 RIP: Advertising IPv6 routes from %s (%s): %s", parentIP, RipManager.ParentIf, routeStr[0:len(routeStr)-2])
	}
}

// Close is called when network is deleted. Shuts down ripRouter goroutines and
// sends out the last update, setting all route metrics to 16
func (RipManager RipManager) Close() {
	if RipManager.IPv4.Enabled {
		// Close channel. ipv4ripRouter routine will quit upon channel closure.
		close(RipManager.IPv4.TriggeredUpdate)

		// Advertise all remaining routes as deleted before quitting
		logrus.Debugf("Ipvlan L3 RIP: Stopping IPv4 RIP router on %s", RipManager.ParentIf)
		RipManager.IPv4.RouteTableMutex.Lock()
		for key, routeTableEntry := range RipManager.IPv4.RouteTable {
			routeTableEntry.Updated = time.Now()
			routeTableEntry.Deleted = true
			RipManager.IPv4.RouteTable[key] = routeTableEntry
		}
		RipManager.sendIpv4RipMessage()
		RipManager.IPv4.RouteTableMutex.Unlock()
	}

	if RipManager.IPv6.Enabled {
		// Close channel. ipv6ripRouter routine will quit upon channel closure.
		close(RipManager.IPv6.TriggeredUpdate)

		// Advertise all remaining routes as deleted before quitting
		logrus.Debugf("Ipvlan L3 RIP: Stopping IPv6 RIP router on %s", RipManager.ParentIf)
		RipManager.IPv6.RouteTableMutex.Lock()
		for key, routeTableEntry := range RipManager.IPv6.RouteTable {
			routeTableEntry.Updated = time.Now()
			routeTableEntry.Deleted = true
			RipManager.IPv6.RouteTable[key] = routeTableEntry
		}
		RipManager.sendIpv6RipMessage()
		RipManager.IPv6.RouteTableMutex.Unlock()
	}

	delete(RipManagers, RipManager.NetworkID)
}
