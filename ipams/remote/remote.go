package remote

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/plugins"
	"github.com/docker/libnetwork/ipamapi"
	"github.com/docker/libnetwork/ipams/remote/api"
)

type allocator struct {
	endpoint *plugins.Client
	name     string
}

// PluginResponse is the interface for the plugin request responses
type PluginResponse interface {
	IsSuccess() bool
	GetError() string
}

func newAllocator(name string, client *plugins.Client) (ipamapi.Config, ipamapi.Allocator) {
	a := &allocator{name: name, endpoint: client}
	return a, a
}

// Init registers a remote ipam when its plugin is activated
func Init(cb ipamapi.Callback, n interface{}) error {
	plugins.Handle(ipamapi.NetworkPluginEndpointType, func(name string, client *plugins.Client) {
		ic, ia := newAllocator(name, client)
		if err := cb.RegisterIpam(name, ic, ia); err != nil {
			log.Errorf("error registering remote ipam %s due to %v", name, err)
		}
	})
	return nil
}

func (a *allocator) call(methodName string, arg interface{}, retVal PluginResponse) error {
	method := ipamapi.NetworkPluginEndpointType + "." + methodName
	err := a.endpoint.Call(method, arg, retVal)
	if err != nil {
		return err
	}
	if !retVal.IsSuccess() {
		return fmt.Errorf("remote: %s", retVal.GetError())
	}
	return nil
}

// AddSubnet adds a subnet to the specified address space
func (a *allocator) AddSubnet(addressSpace string, subnet *net.IPNet) error {
	req := &api.AddSubnetRequest{AddressSpace: addressSpace, Subnet: subnet}
	res := &api.AddSubnetResponse{}
	return a.call("AddSubnet", req, res)
}

// RemoveSubnet removes a subnet from the specified address space
func (a *allocator) RemoveSubnet(addressSpace string, subnet *net.IPNet) error {
	req := &api.RemoveSubnetRequest{AddressSpace: addressSpace, Subnet: subnet}
	res := &api.RemoveSubnetResponse{}
	return a.call("RemoveSubnet", req, res)
}

// Request address from the specified address space
func (a *allocator) Request(addressSpace string, subnet *net.IPNet, address net.IP) (net.IP, error) {
	return a.request("RequestAddress", addressSpace, subnet, address)
}

// RequestV6 ipv6 address from the specified address space
func (a *allocator) RequestV6(addressSpace string, subnet *net.IPNet, address net.IP) (net.IP, error) {
	return a.request("RequestAddressV6", addressSpace, subnet, address)
}

func (a *allocator) request(method, addressSpace string, subnet *net.IPNet, address net.IP) (net.IP, error) {
	req := &api.RequestAddress{AddressSpace: addressSpace, Subnet: subnet, Address: address}
	res := &api.RequestAddressResponse{}
	if err := a.call(method, req, res); err != nil {
		return nil, err
	}
	return res.Address, nil
}

// Release the address from the specified address space
func (a *allocator) Release(addressSpace string, address net.IP) {
	req := &api.ReleaseAddressRequest{AddressSpace: addressSpace, Address: address}
	res := &api.ReleaseAddressResponse{}
	if err := a.call("ReleaseAddress", req, res); err != nil {
		log.Warnf("Failed to release address %s on address space %s: %v", address.String(), addressSpace, err)
	}
}
