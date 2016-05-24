package ipvlan

import (
	"net"
	"sync"

	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/discoverapi"
	"github.com/docker/libnetwork/driverapi"
	"github.com/docker/libnetwork/osl"
	"github.com/docker/libnetwork/types"
)

const (
	vethLen             = 7
	containerVethPrefix = "eth"
	vethPrefix          = "veth"
	ipvlanType          = "ipvlan"           // driver type name
	modeL2              = "l2"               // ipvlan mode l2 is the default
	modeL3              = "l3"               // ipvlan L3 mode
	parentOpt           = "parent"           // parent interface -o parent
	modeOpt             = "_mode"            // ipvlan mode ux opt suffix
	bgpNeighborOpt      = "bgp-neighbor"     // BGP neighbor address
	vrfOpt              = "vrf"              // BGP vrf ID
	asOpt               = "asnum"            // BGP AS number default 65000
	remoteAsOpt         = "rasnum"           // BGP remote AS number dafault 65000
	subnetAdvertise     = "subnet-advertise" // Advertise IP Subnet with BGP
)

var driverModeOpt = ipvlanType + modeOpt // mode -o ipvlan_mode

type endpointTable map[string]*endpoint

type networkTable map[string]*network

type driver struct {
	networks networkTable
	sync.Once
	sync.Mutex
	store datastore.DataStore
}

type endpoint struct {
	id       string
	nid      string
	mac      net.HardwareAddr
	addr     *net.IPNet
	addrv6   *net.IPNet
	srcName  string
	dbIndex  uint64
	dbExists bool
}

type network struct {
	id        string
	sbox      osl.Sandbox
	endpoints endpointTable
	driver    *driver
	config    *configuration
	sync.Mutex
}

// Init initializes and registers the libnetwork ipvlan driver
func Init(dc driverapi.DriverCallback, config map[string]interface{}) error {
	c := driverapi.Capability{
		DataScope: datastore.LocalScope,
	}
	d := &driver{
		networks: networkTable{},
	}
	d.initStore(config)

	return dc.RegisterDriver(ipvlanType, d, c)
}

func (d *driver) NetworkAllocate(id string, option map[string]string, ipV4Data, ipV6Data []driverapi.IPAMData) (map[string]string, error) {
	return nil, types.NotImplementedErrorf("not implemented")
}

func (d *driver) NetworkFree(id string) error {
	return types.NotImplementedErrorf("not implemented")
}

func (d *driver) EndpointOperInfo(nid, eid string) (map[string]interface{}, error) {
	return make(map[string]interface{}, 0), nil
}

func (d *driver) Type() string {
	return ipvlanType
}

func (d *driver) ProgramExternalConnectivity(nid, eid string, options map[string]interface{}) error {
	return nil
}

func (d *driver) RevokeExternalConnectivity(nid, eid string) error {
	return nil
}

// DiscoverNew is a notification for a new discovery event.
func (d *driver) DiscoverNew(dType discoverapi.DiscoveryType, data interface{}) error {
	return nil
}

// DiscoverDelete is a notification for a discovery delete event.
func (d *driver) DiscoverDelete(dType discoverapi.DiscoveryType, data interface{}) error {
	return nil
}

func (d *driver) EventNotify(etype driverapi.EventType, nid, tableName, key string, value []byte) {
}
