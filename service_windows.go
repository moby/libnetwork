package libnetwork

import "net"

func (n *network) addLBBackend(ip, vip net.IP, fwMark uint32, ingressPorts []*PortConfig) {
}

func (n *network) rmLBBackend(ip, vip net.IP, lb *loadBalancer, ingressPorts []*PortConfig, rmService bool, fullRemove bool) {
}

func (sb *sandbox) populateLoadbalancers(ep *endpoint) {
}

func arrangeIngressFilterRule() {
}
