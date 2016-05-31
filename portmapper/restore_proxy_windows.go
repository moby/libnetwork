//+build windows

package portmapper

import (
	"net"
)

type proxyRestored struct {
}

func newProxyRestored(proto string, hostIP net.IP, hostPort int, containerIP net.IP, containerPort int) (userlandProxy, error) {
	return nil, nil
}

func (p *proxyRestored) Start() error {
	return nil
}

func (p *proxyRestored) Stop() error {
	return nil
}
