package common

import (
	"time"

	networkdbtypes "github.com/docker/libnetwork/networkdb/types"
)

// Network represents a logical connectivity zone that containers may
// join using the Link method. A Network is managed by a specific driver.
type Network interface {
	// A user chosen name for this network.
	Name() string

	// A system generated id for this network.
	ID() string

	// The type of network, which corresponds to its managing driver.
	Type() string

	// Create a new endpoint to this network symbolically identified by the
	// specified unique name. The options parameter carries driver specific options.
	CreateEndpoint(name string, options ...EndpointOption) (Endpoint, error)

	// Delete the network.
	Delete() error

	// Endpoints returns the list of Endpoint(s) in this network.
	Endpoints() []Endpoint

	// WalkEndpoints uses the provided function to walk the Endpoints
	WalkEndpoints(walker EndpointWalker)

	// EndpointByName returns the Endpoint which has the passed name. If not found, the error ErrNoSuchEndpoint is returned.
	EndpointByName(name string) (Endpoint, error)

	// EndpointByID returns the Endpoint which has the passed id. If not found, the error ErrNoSuchEndpoint is returned.
	EndpointByID(id string) (Endpoint, error)

	// Return certain operational data belonging to this network
	Info() NetworkInfo
}

// NetworkInfo returns some configuration and operational information about the network
type NetworkInfo interface {
	IpamConfig() (string, map[string]string, []*IpamConf, []*IpamConf)
	IpamInfo() ([]*IpamInfo, []*IpamInfo)
	DriverOptions() map[string]string
	Scope() string
	IPv6Enabled() bool
	Internal() bool
	Labels() map[string]string
	Dynamic() bool
	Created() time.Time
	// Peers returns a slice of PeerInfo structures which has the information about the peer
	// nodes participating in the same overlay network. This is currently the per-network
	// gossip cluster. For non-dynamic overlay networks and bridge networks it returns an
	// empty slice
	Peers() []networkdbtypes.PeerInfo
}
