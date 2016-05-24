package ipvlan

var routemanager routingInterface

type host struct {
	isself  bool
	Address string
}

type routingInterface interface {
	CreateVrfNetwork(ParentIface string, vrfID string) error
	AdvertiseNewRoute(localPrefix string, vrfID string) error
	WithdrawRoute(localPrefix string, vrfID string) error
	DiscoverNew(isself bool, Address string) error
	DiscoverDelete(isself bool, Address string) error
}

// InitRouteMonitering initialize and start maniternig routing table of host
func InitRouteMonitering(as string, ras string) {
	routemanager = NewBgpRouteManager(as, ras)
}
