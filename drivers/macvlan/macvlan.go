package macvlan

import (
	"fmt"
	"net"
	"sync"

	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/discoverapi"
	"github.com/docker/libnetwork/driverapi"
	"github.com/docker/libnetwork/netlabel"
	"github.com/docker/libnetwork/osl"
	"github.com/docker/libnetwork/types"
)

const (
	vethLen             = 7
	containerVethPrefix = "eth"
	vethPrefix          = "veth"
	macvlanType         = "macvlan"                           // driver type name
	modePrivate         = "private"                           // macvlan mode private
	modeVepa            = "vepa"                              // macvlan mode vepa
	modeBridge          = "bridge"                            // macvlan mode bridge
	modePassthru        = "passthru"                          // macvlan mode passthrough
	parentOpt           = "parent"                            // parent interface -o parent
	modeOpt             = "_mode"                             // macvlan mode ux opt suffix
	parentFile          = "parent-file"                       // parent file
	globalScope         = "global"                            // local scope label data
	localScope          = "local"                             // local scope label data
	driverPrefix        = "com.docker.network.driver.macvlan" // driver prefix used for labels
	driverScopeLabel    = driverPrefix + ".scope"
)

var driverModeOpt = macvlanType + modeOpt // mode --option macvlan_mode

type endpointTable map[string]*endpoint

type networkTable map[string]*network

type driver struct {
	networks networkTable
	scope    string
	sync.Once
	sync.Mutex
	store datastore.DataStore
}

type endpoint struct {
	id      string
	mac     net.HardwareAddr
	addr    *net.IPNet
	addrv6  *net.IPNet
	srcName string
}

type network struct {
	id        string
	sbox      osl.Sandbox
	endpoints endpointTable
	driver    *driver
	config    *configuration
	once      *sync.Once
	sync.Mutex
	dbExists bool
	dbIndex  uint64
}

// Init registers a new instance of the macvlan driver
func Init(dc driverapi.DriverCallback, config map[string]interface{}) error {
	d := &driver{
		networks: networkTable{},
	}
	// register the driver as a locally scoped if a local scope label is passed
	if labelData, ok := config[driverScopeLabel]; ok {
		if labelData == localScope {
			// register the driver as locally scoped if no scope label is passed
			c := driverapi.Capability{
				DataScope: datastore.LocalScope,
			}
			d.scope = localScope
			// initialize the local boltdb persistent datastore
			d.initStore(config)

			return dc.RegisterDriver(macvlanType, d, c)
		}
	}
	// default to a globally scoped driver
	c := driverapi.Capability{
		DataScope: datastore.GlobalScope,
	}
	if data, ok := config[netlabel.GlobalKVClient]; ok {
		var err error
		dsc, ok := data.(discoverapi.DatastoreConfigData)
		if !ok {
			return types.InternalErrorf("incorrect data in datastore configuration: %v", data)
		}
		d.store, err = datastore.NewDataStoreFromConfig(dsc)
		if err != nil {
			return types.InternalErrorf("failed to initialize data store: %v", err)
		}
	}
	d.scope = globalScope

	return dc.RegisterDriver(macvlanType, d, c)
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
	return macvlanType
}

func (d *driver) ProgramExternalConnectivity(nid, eid string, options map[string]interface{}) error {
	return nil
}

func (d *driver) RevokeExternalConnectivity(nid, eid string) error {
	return nil
}

// DiscoverNew is a notification for a new discovery event
func (d *driver) DiscoverNew(dType discoverapi.DiscoveryType, data interface{}) error {
	switch dType {
	case discoverapi.NodeDiscovery:
		nodeData, ok := data.(discoverapi.NodeDiscoveryData)
		if !ok || nodeData.Address == "" {
			return fmt.Errorf("invalid discovery data")
		}
	case discoverapi.DatastoreConfig:
		var err error
		if d.store != nil {
			return types.ForbiddenErrorf("cannot accept datastore configuration: macvlan driver has a datastore configured already")
		}
		dsc, ok := data.(discoverapi.DatastoreConfigData)
		if !ok {
			return types.InternalErrorf("incorrect data in datastore configuration: %v", data)
		}
		d.store, err = datastore.NewDataStoreFromConfig(dsc)
		if err != nil {
			return types.InternalErrorf("failed to initialize data store: %v", err)
		}
	default:
	}
	return nil
}

// DiscoverDelete is a notification for a discovery delete event
func (d *driver) DiscoverDelete(dType discoverapi.DiscoveryType, data interface{}) error {
	return nil
}

func (d *driver) EventNotify(etype driverapi.EventType, nid, tableName, key string, value []byte) {
}
