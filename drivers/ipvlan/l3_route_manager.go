package ipvlan

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	bgpapi "github.com/osrg/gobgp/libapi"
	bgp "github.com/osrg/gobgp/packet/bgp"
	bgpserver "github.com/osrg/gobgp/server"
	"strconv"
	"strings"
)

const (
	bgpVrfprefix = "vrf"
)

type rib struct {
	path  *bgpapi.Path
	vrfID string
}

// BgpRouteManager advertize and withdraw routes by BGP
type BgpRouteManager struct {
	// Master interface for IPVlan and BGP peering source
	parentIfaces  map[string]string
	bgpServer     *bgpserver.BgpServer
	learnedRoutes map[string]*ribLocal
	bgpGlobalcfg  *bgpapi.Global
	asnum         int
	rasnum        int
	neighborlist  []string
}

// NewBgpRouteManager initialize route manager
func NewBgpRouteManager(as string, ras string) *BgpRouteManager {
	a, err := strconv.Atoi(as)
	if err != nil {
		log.Debugf("AS number is not numeral or empty %s, using default AS num: 65000", as)
		a = 65000
	}
	ra, err := strconv.Atoi(ras)
	if err != nil {
		log.Debugf("Remote AS number is not numeral or empty %s, using default remote AS num: 65000", as)
		ra = 65000
	}
	b := &BgpRouteManager{
		parentIfaces:  make(map[string]string),
		asnum:         a,
		rasnum:        ra,
		learnedRoutes: make(map[string]*ribLocal),
	}
	b.bgpServer = bgpserver.NewBgpServer()
	go b.bgpServer.Serve()
	return b
}

//CreateVrfNetwork create vrf in BGP server and start monitoring vrf rib if vrfID="", network use global rib
func (b *BgpRouteManager) CreateVrfNetwork(parentIface string, vrfID string) error {
	log.Debugf("BGP Global config : %v", b.bgpGlobalcfg)
	if b.bgpGlobalcfg == nil {
		log.Debugf("BGP Global config is emply set config")
		b.parentIfaces["global"] = parentIface
		iface, _ := net.InterfaceByName(parentIface)
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil {
				log.Errorf("error deriving ip-address in bind interface (%s) : %v", parentIface, err)
				return err
			}
			if ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			RouterID := ip.String()
			b.SetBgpConfig(RouterID)
			break
		}
	}
	if vrfID == "" {
		log.Debugf("vrf ID is empty. network paths are in global rib")
		return nil
	}
	b.parentIfaces[vrfID] = parentIface
	err := cleanExistingRoutes(parentIface)
	if err != nil {
		log.Debugf("Error cleaning old routes: %s", err)
		return err
	}
	rdrtstr := strconv.Itoa(b.asnum) + ":" + vrfID
	rd, err := bgp.ParseRouteDistinguisher(rdrtstr)
	if err != nil {
		log.Errorf("Fail to parse RD from vrfID %s : %s", vrfID, err)
		return err
	}
	rdSerialized, _ := rd.Serialize()

	rt, err := bgp.ParseRouteTarget(rdrtstr)
	if err != nil {
		log.Errorf("Fail to parse RT from vrfID %s : %s", vrfID, err)
		return err
	}
	rtSerialized, _ := rt.Serialize()
	var rts [][]byte
	rts = append(rts, rtSerialized)

	arg := &bgpapi.Vrf{
		Name:     bgpVrfprefix + vrfID,
		Rd:       rdSerialized,
		ImportRt: rts,
		ExportRt: rts,
	}
	err = b.bgpServer.AddVrf(arg)
	if err != nil {
		return err
	}
	go func() {
		err := b.monitorBestPath(vrfID)
		log.Errorf("faital monitoring VrfID %s rib : %v", vrfID, err)
	}()
	return nil
}

//SetBgpConfig set routet id
func (b *BgpRouteManager) SetBgpConfig(RouterID string) error {
	b.bgpGlobalcfg = &bgpapi.Global{As: uint32(b.asnum), RouterId: RouterID}
	err := b.bgpServer.StartServer(&bgpapi.StartServerRequest{
		Global: b.bgpGlobalcfg,
	})
	if err != nil {
		return err
	}
	go func() {
		err := b.monitorBestPath("")
		log.Errorf("faital monitoring global rib : %v", err)
	}()
	log.Debugf("Set BGP Global config: as %d, router id %v", b.asnum, RouterID)
	return nil
}

func (b *BgpRouteManager) handleRibUpdate(p *rib) error {
	bgpCache := &ribCache{
		BgpTable: make(map[string]*ribLocal),
	}
	monitorUpdate, err := bgpCache.handleBgpRibMonitor(p.path, p.vrfID)
	if err != nil {
		log.Errorf("error processing bgp update [ %s ]", err)
		return err
	} else if monitorUpdate == nil {
		return nil
	}
	if monitorUpdate.IsLocal != true {
		if p.path.IsWithdraw {
			monitorUpdate.IsWithdraw = true
			log.Debugf("BGP update has withdrawn the routes:  %v ", monitorUpdate)
			if route, ok := b.learnedRoutes[monitorUpdate.BgpPrefix.String()]; ok {
				err = delNetlinkRoute(route.BgpPrefix, route.NextHop, b.parentIfaces[p.vrfID])
				// If the bgp update contained a withdraw, remove the local netlink route for the remote endpoint
				if err != nil {
					log.Errorf("Error removing learned bgp route [ %s ]", err)
					return err
				}
				delete(b.learnedRoutes, monitorUpdate.BgpPrefix.String())
			} else {
				log.Debugf("Update withdrawn route has Unknown BGP prefix %s", monitorUpdate.BgpPrefix.String())
				return nil
			}
		} else {
			monitorUpdate.IsWithdraw = false
			b.learnedRoutes[monitorUpdate.BgpPrefix.String()] = monitorUpdate
			log.Debugf("Learned routes: %v ", monitorUpdate)

			err = addNetlinkRoute(monitorUpdate.BgpPrefix, monitorUpdate.NextHop, b.parentIfaces[p.vrfID])
			if err != nil {
				log.Debugf("Error Adding route results [ %s ]", err)
				return err
			}
			log.Debugf("Updated the local prefix cache from the newly learned BGP update:")
			for n, entry := range b.learnedRoutes {
				log.Debugf("%s - %+v", n, entry)
			}
		}
	}
	log.Debugf("Verbose update details: %s", monitorUpdate)
	return nil
}

func (b *BgpRouteManager) monitorBestPath(VrfID string) error {
	var routeFamily uint32
	if VrfID == "" {
		routeFamily = uint32(bgp.RF_IPv4_UC)
		VrfID = "global"
	} else {
		routeFamily = uint32(bgp.RF_IPv4_VPN)
	}
	err := func() error {
		currib, err := b.bgpServer.GetRib(&bgpapi.Table{Type: bgpapi.Resource_GLOBAL, Family: routeFamily})
		if err != nil {
			return err
		}
		for _, d := range currib.Destinations {
			for _, p := range d.Paths {
				if p.Best {
					b.handleRibUpdate(&rib{path: p, vrfID: VrfID})
					break
				}
			}
		}
		return nil
	}()

	if err != nil {
		return err
	}
	ribCh, EndCh, err := b.bgpServer.MonitorRib(&bgpapi.Table{Type: bgpapi.Resource_ADJ_IN, Family: routeFamily})
	if err != nil {
		return err
	}
	for r := range ribCh {
		if err := r.Err(); err != nil {
			log.Debug(err.Error())
			EndCh <- struct{}{}
			return err
		}
		err := b.handleRibUpdate(&rib{path: r.Data.(*bgpapi.Destination).Paths[0], vrfID: VrfID})
		if err != nil {
			log.Errorf(err.Error())
			continue
		}
	}
	return nil
}

// AdvertiseNewRoute advetise the local namespace IP prefixes to the bgp neighbors
func (b *BgpRouteManager) AdvertiseNewRoute(localPrefix string, VrfID string) error {
	_, localPrefixCIDR, _ := net.ParseCIDR(localPrefix)
	log.Debugf("Adding this hosts container network [ %s ] into the BGP domain", localPrefix)
	path := &bgpapi.Path{
		Pattrs:     make([][]byte, 0),
		IsWithdraw: false,
	}
	var target bgpapi.Resource
	if VrfID == "" {
		target = bgpapi.Resource_GLOBAL
	} else {
		target = bgpapi.Resource_VRF
	}
	localPrefixMask, _ := localPrefixCIDR.Mask.Size()
	path.Nlri, _ = bgp.NewIPAddrPrefix(uint8(localPrefixMask), localPrefixCIDR.IP.String()).Serialize()
	n, _ := bgp.NewPathAttributeNextHop("0.0.0.0").Serialize()
	path.Pattrs = append(path.Pattrs, n)
	origin, _ := bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP).Serialize()
	path.Pattrs = append(path.Pattrs, origin)
	arg := &bgpapi.AddPathRequest{
		Resource: target,
		VrfId:    bgpVrfprefix + VrfID,
		Path:     path,
	}
	_, err := b.bgpServer.AddPath(arg)
	if err != nil {
		return err
	}
	return nil
}

//WithdrawRoute withdraw the local namespace IP prefixes to the bgp neighbors
func (b *BgpRouteManager) WithdrawRoute(localPrefix string, VrfID string) error {
	_, localPrefixCIDR, _ := net.ParseCIDR(localPrefix)
	log.Debugf("Withdraw this hosts container network [ %s ] from the BGP domain", localPrefix)
	path := &bgpapi.Path{
		Pattrs:     make([][]byte, 0),
		IsWithdraw: true,
	}
	var target bgpapi.Resource
	if VrfID == "" {
		target = bgpapi.Resource_GLOBAL
	} else {
		target = bgpapi.Resource_VRF
	}
	localPrefixMask, _ := localPrefixCIDR.Mask.Size()
	path.Nlri, _ = bgp.NewIPAddrPrefix(uint8(localPrefixMask), localPrefixCIDR.IP.String()).Serialize()
	n, _ := bgp.NewPathAttributeNextHop("0.0.0.0").Serialize()
	path.Pattrs = append(path.Pattrs, n)
	origin, _ := bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP).Serialize()
	path.Pattrs = append(path.Pattrs, origin)
	arg := &bgpapi.DeletePathRequest{
		Resource: target,
		VrfId:    bgpVrfprefix + VrfID,
		Path:     path,
	}
	err := b.bgpServer.DeletePath(arg)
	if err != nil {
		return err
	}
	return nil
}

//ModPeer add or delete bgp peer : oreration add - true, del - fasle
func (b *BgpRouteManager) ModPeer(peeraddr string, operation bool) error {
	peer := &bgpapi.Peer{
		Conf: &bgpapi.PeerConf{
			NeighborAddress: peeraddr,
			PeerAs:          uint32(b.rasnum),
		},
		Families: []uint32{uint32(bgp.RF_IPv4_UC), uint32(bgp.RF_IPv6_UC), uint32(bgp.RF_IPv4_VPN), uint32(bgp.RF_IPv6_VPN)},
	}
	if operation {
		if err := b.bgpServer.AddNeighbor(peer); err != nil {
			return err
		}
	} else {
		if err := b.bgpServer.DeleteNeighbor(peer); err != nil {
			return err
		}
	}
	return nil
}

// DiscoverNew host discovery for add new peer
func (b *BgpRouteManager) DiscoverNew(isself bool, Address string) error {
	if isself {
		error := b.SetBgpConfig(Address)
		if error != nil {
			return error
		}
		for _, nAddr := range b.neighborlist {
			log.Debugf("BGP neighbor add %s", nAddr)
			error := b.ModPeer(nAddr, true)
			if error != nil {
				return error
			}
		}
	} else {
		if b.bgpGlobalcfg != nil {
			log.Debugf("BGP neighbor add %s", Address)
			error := b.ModPeer(Address, true)
			if error != nil {
				return error
			}
		}
		b.neighborlist = append(b.neighborlist, Address)
	}
	return nil
}

//DiscoverDelete host discovery for delete
func (b *BgpRouteManager) DiscoverDelete(isself bool, Address string) error {
	if isself {
		return nil
	}
	if b.bgpGlobalcfg != nil {
		log.Debugf("BGP neighbor del %s", Address)
		error := b.ModPeer(Address, false)
		if error != nil {
			return error
		}
	}

	return nil
}
func (cache *ribCache) handleBgpRibMonitor(routeMonitor *bgpapi.Path, VrfID string) (*ribLocal, error) {
	ribLocal := &ribLocal{}
	var nlri bgp.AddrPrefixInterface

	if len(routeMonitor.Nlri) > 0 {
		if VrfID == "global" {
			nlri = &bgp.IPAddrPrefix{}
			err := nlri.DecodeFromBytes(routeMonitor.Nlri)
			if err != nil {
				log.Errorf("Error parsing the bgp update nlri")
				return nil, err
			}
			bgpPrefix, err := parseIPNet(nlri.String())
			if err != nil {
				log.Errorf("Error parsing the bgp update prefix: %s", nlri.String())
				return nil, err
			}
			ribLocal.BgpPrefix = bgpPrefix

		} else {
			nlri = bgp.NewLabeledVPNIPAddrPrefix(24, "", *bgp.NewMPLSLabelStack(), nil)
			err := nlri.DecodeFromBytes(routeMonitor.Nlri)
			if err != nil {
				log.Errorf("Error parsing the bgp update nlri")
				return nil, err
			}
			nlriSplit := strings.Split(nlri.String(), ":")
			if VrfID != nlriSplit[1] {
				return nil, nil
			}
			bgpPrefix, err := parseIPNet(nlriSplit[len(nlriSplit)-1])
			if err != nil {
				log.Errorf("Error parsing the bgp update vpn prefix: %s", nlriSplit[len(nlriSplit)-1])
				return nil, err
			}
			ribLocal.BgpPrefix = bgpPrefix

		}
	}
	log.Debugf("BGP update for prefix: [ %s ] ", nlri.String())
	for _, attr := range routeMonitor.Pattrs {
		p, err := bgp.GetPathAttribute(attr)
		if err != nil {
			log.Errorf("Error parsing the bgp update attr")
			return nil, err
		}
		err = p.DecodeFromBytes(attr)
		if err != nil {
			log.Errorf("Error parsing the bgp update attr")
			return nil, err
		}
		log.Debugf("Type: [ %d ] ,Value [ %s ]", p.GetType(), p.String())
		switch p.GetType() {
		case bgp.BGP_ATTR_TYPE_ORIGIN:
			// 0 = iBGP; 1 = eBGP
			if p.(*bgp.PathAttributeOrigin).Value != nil {
				log.Debugf("Type Code: [ %d ] Origin: %s", bgp.BGP_ATTR_TYPE_ORIGIN, p.(*bgp.PathAttributeOrigin).String())
			}
		case bgp.BGP_ATTR_TYPE_AS_PATH:
			if p.(*bgp.PathAttributeAsPath).Value != nil {
				log.Debugf("Type Code: [ %d ] AS_Path: %s", bgp.BGP_ATTR_TYPE_AS_PATH, p.String())
			}
		case bgp.BGP_ATTR_TYPE_NEXT_HOP:
			if p.(*bgp.PathAttributeNextHop).Value.String() != "" {
				log.Debugf("Type Code: [ %d ] Nexthop: %s", bgp.BGP_ATTR_TYPE_NEXT_HOP, p.String())
				n := p.(*bgp.PathAttributeNextHop)
				ribLocal.NextHop = n.Value
				if ribLocal.NextHop.String() == "0.0.0.0" {
					ribLocal.IsLocal = true
				}
			}
		case bgp.BGP_ATTR_TYPE_MULTI_EXIT_DISC:
			if p.(*bgp.PathAttributeMultiExitDisc).Value >= 0 {
				log.Debugf("Type Code: [ %d ] MED: %g", bgp.BGP_ATTR_TYPE_MULTI_EXIT_DISC, p.String())
			}
		case bgp.BGP_ATTR_TYPE_LOCAL_PREF:
			if p.(*bgp.PathAttributeLocalPref).Value >= 0 {
				log.Debugf("Type Code: [ %d ] Local Pref: %g", bgp.BGP_ATTR_TYPE_LOCAL_PREF, p.String())
			}
		case bgp.BGP_ATTR_TYPE_ORIGINATOR_ID:
			if p.(*bgp.PathAttributeOriginatorId).Value != nil {
				log.Debugf("Type Code: [ %d ] Originator IP: %s", bgp.BGP_ATTR_TYPE_ORIGINATOR_ID, p.String())
				ribLocal.OriginatorIP = p.(*bgp.PathAttributeOriginatorId).Value
				log.Debugf("Type Code: [ %d ] Originator IP: %s", bgp.BGP_ATTR_TYPE_ORIGINATOR_ID, ribLocal.OriginatorIP)
			}
		case bgp.BGP_ATTR_TYPE_CLUSTER_LIST:
			if len(p.(*bgp.PathAttributeClusterList).Value) > 0 {
				log.Debugf("Type Code: [ %d ] Cluster List: %s", bgp.BGP_ATTR_TYPE_CLUSTER_LIST, p.String())
			}
		case bgp.BGP_ATTR_TYPE_MP_REACH_NLRI:
			if p.(*bgp.PathAttributeMpReachNLRI).Value != nil {
				log.Debugf("Type Code: [ %d ] MP Reachable: %s", bgp.BGP_ATTR_TYPE_MP_REACH_NLRI, p.String())
				mpreach := p.(*bgp.PathAttributeMpReachNLRI)
				if len(mpreach.Value) != 1 {
					log.Errorf("include only one route in mp_reach_nlri")
				}
				nlri = mpreach.Value[0]
				ribLocal.NextHop = mpreach.Nexthop
				if ribLocal.NextHop.String() == "0.0.0.0" {
					ribLocal.IsLocal = true
				}
			}
		case bgp.BGP_ATTR_TYPE_MP_UNREACH_NLRI:
			if p.(*bgp.PathAttributeMpUnreachNLRI).Value != nil {
				log.Debugf("Type Code: [ %d ]  MP Unreachable: %v", bgp.BGP_ATTR_TYPE_MP_UNREACH_NLRI, p.String())
			}
		case bgp.BGP_ATTR_TYPE_EXTENDED_COMMUNITIES:
			if p.(*bgp.PathAttributeExtendedCommunities).Value != nil {
				log.Debugf("Type Code: [ %d ] Extended Communities: %v", bgp.BGP_ATTR_TYPE_EXTENDED_COMMUNITIES, p.String())
			}
		default:
			log.Errorf("Unknown BGP attribute code [ %d ]", p.GetType())
		}
	}
	return ribLocal, nil

}

// return string representation of pluginConfig for debugging
func (d *ribLocal) stringer() string {
	str := fmt.Sprintf("Prefix:[ %s ], ", d.BgpPrefix.String())
	str = str + fmt.Sprintf("OriginatingIP:[ %s ], ", d.OriginatorIP.String())
	str = str + fmt.Sprintf("Nexthop:[ %s ], ", d.NextHop.String())
	str = str + fmt.Sprintf("IsWithdrawn:[ %t ], ", d.IsWithdraw)
	str = str + fmt.Sprintf("IsHostRoute:[ %t ]", d.IsHostRoute)
	return str
}

func parseIPNet(s string) (*net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	return &net.IPNet{IP: ip, Mask: ipNet.Mask}, nil
}
