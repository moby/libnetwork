// +build !linux,!windows

package libnetwork

import (
	"fmt"
	"net"

	"github.com/docker/libnetwork/types"
)

func (c *controller) cleanupServiceBindings(nid string) {
}

func (c *controller) addServiceBinding(name, sid, nid, eid string, vip net.IP, ingressPorts []*types.PortConfig, aliases []string, ip net.IP) error {
	return fmt.Errorf("not supported")
}

func (c *controller) rmServiceBinding(name, sid, nid, eid string, vip net.IP, ingressPorts []*types.PortConfig, aliases []string, ip net.IP) error {
	return fmt.Errorf("not supported")
}

func (sb *sandbox) populateLoadbalancers(ep *endpoint) {
}

func arrangeIngressFilterRule() {
}
