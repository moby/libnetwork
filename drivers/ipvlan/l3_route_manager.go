package ipvlan

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/osrg/gobgp/config"
	bgp "github.com/osrg/gobgp/packet/bgp"
	bgpserver "github.com/osrg/gobgp/server"
	"github.com/osrg/gobgp/table"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	bgpVrfprefix = "vrf"
)

type rib struct {
	path  *table.Path
	vpnID string
}

// BgpRouteManager advertize and withdraw routes by BGP
type BgpRouteManager struct {
	// Master interface for IPVlan and BGP peering source
	parentIfaces  map[string]string
	bgpServer     *bgpserver.BgpServer
	learnedRoutes map[string]*ribLocal
	bgpGlobalcfg  *config.Global
	asnum         int
	rasnum        int
	neighborlist  []string
	pathUuidlist  map[string][]byte
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
		pathUuidlist:  make(map[string][]byte),
	}
	b.bgpServer = bgpserver.NewBgpServer()
	go b.bgpServer.Serve()
	return b
}

//CreateVrfNetwork create vrf in BGP server and start monitoring vrf rib if vpnID="", network use global rib
func (b *BgpRouteManager) CreateVrfNetwork(parentIface string, vpnID string) error {
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
	if vpnID == "" {
		log.Debugf("vrf ID is empty. network paths are in global rib")
		return nil
	}
	b.parentIfaces[vpnID] = parentIface
	err := cleanExistingRoutes(parentIface)
	if err != nil {
		log.Debugf("Error cleaning old routes: %s", err)
		return err
	}
	rdrtstr := strconv.Itoa(b.asnum) + ":" + vpnID
	rd, err := bgp.ParseRouteDistinguisher(rdrtstr)
	if err != nil {
		log.Errorf("Fail to parse RD from vpnID %s : %s", vpnID, err)
		return err
	}

	rt, err := bgp.ParseRouteTarget(rdrtstr)
	if err != nil {
		log.Errorf("Fail to parse RT from vpnID %s : %s", vpnID, err)
		return err
	}
	var rts []bgp.ExtendedCommunityInterface
	rts = append(rts, rt)
	err = b.bgpServer.AddVrf(
		bgpVrfprefix+vpnID,
		rd,
		rts,
		rts)
	if err != nil {
		return err
	}
	//	go func() {
	//		err := b.monitorBestPath(vpnID)
	//		log.Errorf("faital monitoring VpnID %s rib : %v", vpnID, err)
	//	}()
	return nil
}

//SetBgpConfig set routet id
func (b *BgpRouteManager) SetBgpConfig(RouterID string) error {
	b.bgpGlobalcfg = &config.Global{
		Config: config.GlobalConfig{
			As:       uint32(b.asnum),
			RouterId: RouterID,
		},
	}
	err := b.bgpServer.Start(
		b.bgpGlobalcfg,
	)
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
	monitorUpdate, err := bgpCache.handleBgpRibMonitor(p.path, p.vpnID)
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
				err = delNetlinkRoute(route.BgpPrefix, route.NextHop, b.parentIfaces[p.vpnID])
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

			err = addNetlinkRoute(monitorUpdate.BgpPrefix, monitorUpdate.NextHop, b.parentIfaces[p.vpnID])
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

func (b *BgpRouteManager) monitorBestPath(VpnID string) error {
	w := b.bgpServer.Watch(bgpserver.WatchBestPath())
	for {
		select {
		case ev := <-w.Event():
			switch msg := ev.(type) {
			case *bgpserver.WatchEventBestPath:
				for _, path := range msg.PathList {
					vpnid := "global"
					log.Debugf("Update BGP path %v", path)
					for _, attr := range path.GetPathAttrs() {
						if attr.GetType() == bgp.BGP_ATTR_TYPE_EXTENDED_COMMUNITIES {
							for _, extcomm := range attr.(*bgp.PathAttributeExtendedCommunities).Value {
								extcommStrs := strings.Split(extcomm.String(), ":")
								vpnid = extcommStrs[len(extcommStrs)-1]
							}
						}
					}
					b.handleRibUpdate(&rib{path: path, vpnID: vpnid})
				}
			}
		}
	}
	//	return nil
}

// AdvertiseNewRoute advetise the local namespace IP prefixes to the bgp neighbors
func (b *BgpRouteManager) AdvertiseNewRoute(localPrefix string, VpnID string) error {
	var err error
	log.Debugf("Adding this hosts container network [ %s ] into the BGP domain", localPrefix)
	_, localPrefixCIDR, err := net.ParseCIDR(localPrefix)
	if err != nil {
		return err
	}
	localPrefixMask, _ := localPrefixCIDR.Mask.Size()
	attrs := []bgp.PathAttributeInterface{
		bgp.NewPathAttributeOrigin(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
		bgp.NewPathAttributeNextHop("0.0.0.0"),
	}
	if VpnID == "" {
		var uuid []byte
		uuid, err = b.bgpServer.AddPath("", []*table.Path{table.NewPath(nil, bgp.NewIPAddrPrefix(uint8(localPrefixMask), localPrefixCIDR.IP.String()), false, attrs, time.Now(), false)})
		b.pathUuidlist[localPrefix] = uuid
	} else {
		var uuid []byte
		rdrtstr := strconv.Itoa(b.asnum) + ":" + VpnID
		uuid, err = b.bgpServer.AddPath(bgpVrfprefix+VpnID, []*table.Path{table.NewPath(nil, bgp.NewIPAddrPrefix(uint8(localPrefixMask), localPrefixCIDR.IP.String()), false, attrs, time.Now(), false)})
		b.pathUuidlist[rdrtstr+localPrefix] = uuid
	}
	if err != nil {
		return err
	}
	return nil
}

//WithdrawRoute withdraw the local namespace IP prefixes to the bgp neighbors
func (b *BgpRouteManager) WithdrawRoute(localPrefix string, VpnID string) error {
	var err error
	log.Debugf("Withdraw this hosts container network [ %s ] from the BGP domain", localPrefix)
	if VpnID == "" {
		uuid := b.pathUuidlist[localPrefix]
		err = b.bgpServer.DeletePath(uuid, 0, "", nil)
		delete(b.pathUuidlist, localPrefix)
	} else {
		rdrtstr := strconv.Itoa(b.asnum) + ":" + VpnID
		uuid := b.pathUuidlist[rdrtstr+localPrefix]
		err = b.bgpServer.DeletePath(uuid, 0, bgpVrfprefix+VpnID, nil)
		delete(b.pathUuidlist, rdrtstr+localPrefix)
	}
	if err != nil {
		return err
	}
	return nil
}

//ModPeer add or delete bgp peer : oreration add - true, del - fasle
func (b *BgpRouteManager) ModPeer(peeraddr string, operation bool) error {
	peer := &config.Neighbor{
		Config: config.NeighborConfig{
			NeighborAddress: peeraddr,
			PeerAs:          uint32(b.rasnum),
		},
		AfiSafis: []config.AfiSafi{
			{
				Config: config.AfiSafiConfig{
					AfiSafiName: config.AFI_SAFI_TYPE_IPV4_UNICAST,
					Enabled:     true,
				},
			},
			{
				Config: config.AfiSafiConfig{
					AfiSafiName: config.AFI_SAFI_TYPE_IPV6_UNICAST,
					Enabled:     true,
				},
			},
			{
				Config: config.AfiSafiConfig{
					AfiSafiName: config.AFI_SAFI_TYPE_IPV4_LABELLED_UNICAST,
					Enabled:     true,
				},
			},
			{
				Config: config.AfiSafiConfig{
					AfiSafiName: config.AFI_SAFI_TYPE_IPV6_LABELLED_UNICAST,
					Enabled:     true,
				},
			},
		},
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
func (cache *ribCache) handleBgpRibMonitor(route *table.Path, VpnID string) (*ribLocal, error) {
	ribLocal := &ribLocal{}
	nlri := route.GetNlri()
	if nlri != nil {
		bgpPrefix, err := parseIPNet(nlri.String())
		if err != nil {
			log.Errorf("Error parsing the bgp update prefix: %s", nlri.String())
			return nil, err
		}
		ribLocal.BgpPrefix = bgpPrefix
	}
	log.Debugf("BGP update for prefix: [ %s ] ", nlri.String())
	for _, attr := range route.GetPathAttrs() {
		log.Debugf("Type: [ %d ] ,Value [ %s ]", attr.GetType(), attr.String())
		switch attr.GetType() {
		case bgp.BGP_ATTR_TYPE_ORIGIN:
			// 0 = iBGP; 1 = eBGP
			if attr.(*bgp.PathAttributeOrigin).Value != nil {
				log.Debugf("Type Code: [ %d ] Origin: %s", bgp.BGP_ATTR_TYPE_ORIGIN, attr.(*bgp.PathAttributeOrigin).String())
			}
		case bgp.BGP_ATTR_TYPE_AS_PATH:
			if attr.(*bgp.PathAttributeAsPath).Value != nil {
				log.Debugf("Type Code: [ %d ] AS_Path: %s", bgp.BGP_ATTR_TYPE_AS_PATH, attr.String())
			}
		case bgp.BGP_ATTR_TYPE_NEXT_HOP:
			if attr.(*bgp.PathAttributeNextHop).Value.String() != "" {
				log.Debugf("Type Code: [ %d ] Nexthop: %s", bgp.BGP_ATTR_TYPE_NEXT_HOP, attr.String())
				n := attr.(*bgp.PathAttributeNextHop)
				ribLocal.NextHop = n.Value
				if ribLocal.NextHop.String() == "0.0.0.0" {
					ribLocal.IsLocal = true
				}
			}
		case bgp.BGP_ATTR_TYPE_MULTI_EXIT_DISC:
			if attr.(*bgp.PathAttributeMultiExitDisc).Value >= 0 {
				log.Debugf("Type Code: [ %d ] MED: %g", bgp.BGP_ATTR_TYPE_MULTI_EXIT_DISC, attr.String())
			}
		case bgp.BGP_ATTR_TYPE_LOCAL_PREF:
			if attr.(*bgp.PathAttributeLocalPref).Value >= 0 {
				log.Debugf("Type Code: [ %d ] Local Pref: %g", bgp.BGP_ATTR_TYPE_LOCAL_PREF, attr.String())
			}
		case bgp.BGP_ATTR_TYPE_ORIGINATOR_ID:
			if attr.(*bgp.PathAttributeOriginatorId).Value != nil {
				log.Debugf("Type Code: [ %d ] Originator IP: %s", bgp.BGP_ATTR_TYPE_ORIGINATOR_ID, attr.String())
				ribLocal.OriginatorIP = attr.(*bgp.PathAttributeOriginatorId).Value
				log.Debugf("Type Code: [ %d ] Originator IP: %s", bgp.BGP_ATTR_TYPE_ORIGINATOR_ID, ribLocal.OriginatorIP)
			}
		case bgp.BGP_ATTR_TYPE_CLUSTER_LIST:
			if len(attr.(*bgp.PathAttributeClusterList).Value) > 0 {
				log.Debugf("Type Code: [ %d ] Cluster List: %s", bgp.BGP_ATTR_TYPE_CLUSTER_LIST, attr.String())
			}
		case bgp.BGP_ATTR_TYPE_MP_REACH_NLRI:
			if attr.(*bgp.PathAttributeMpReachNLRI).Value != nil {
				log.Debugf("Type Code: [ %d ] MP Reachable: %s", bgp.BGP_ATTR_TYPE_MP_REACH_NLRI, attr.String())
				mpreach := attr.(*bgp.PathAttributeMpReachNLRI)
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
			if attr.(*bgp.PathAttributeMpUnreachNLRI).Value != nil {
				log.Debugf("Type Code: [ %d ]  MP Unreachable: %v", bgp.BGP_ATTR_TYPE_MP_UNREACH_NLRI, attr.String())
			}
		case bgp.BGP_ATTR_TYPE_EXTENDED_COMMUNITIES:
			if attr.(*bgp.PathAttributeExtendedCommunities).Value != nil {
				log.Debugf("Type Code: [ %d ] Extended Communities: %v", bgp.BGP_ATTR_TYPE_EXTENDED_COMMUNITIES, attr.String())
			}
		default:
			log.Errorf("Unknown BGP attribute code [ %d ]", attr.GetType())
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
