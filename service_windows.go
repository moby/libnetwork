package libnetwork

import (
	"net"

	"github.com/docker/libnetwork/types"
)

func (n *network) addLBBackend(ip, vip net.IP, fwMark uint32, ingressPorts []*types.PortConfig, addService bool) {
}

func (n *network) rmLBBackend(ip, vip net.IP, fwMark uint32, ingressPorts []*types.PortConfig, rmService bool) {
}

func (sb *sandbox) populateLoadbalancers(ep *endpoint) {
}

func arrangeIngressFilterRule() {
}
