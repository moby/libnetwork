package libnetwork

import (
	"github.com/docker/libnetwork/driverapi"
	"github.com/docker/libnetwork/drivers/bridge"
	"github.com/docker/libnetwork/drivers/host"
	"github.com/docker/libnetwork/drivers/null"
	o "github.com/docker/libnetwork/drivers/overlay"
	"github.com/docker/libnetwork/drivers/remote"
	"github.com/docker/libnetwork/ipamapi"
	builtin_ipam "github.com/docker/libnetwork/ipams/builtin"
	remote_ipam "github.com/docker/libnetwork/ipams/remote"
)

func initDrivers(dc driverapi.DriverCallback) error {
	for _, fn := range [](func(driverapi.DriverCallback) error){
		bridge.Init,
		host.Init,
		null.Init,
		remote.Init,
		o.Init,
	} {
		if err := fn(dc); err != nil {
			return err
		}
	}
	return nil
}

func initIpams(ic ipamapi.Callback, ds interface{}) error {
	// Libnetwork ipam will point to remote plugin if registered, otherwise to built-in provider
	for _, fn := range [](func(ipamapi.Callback, interface{}) error){
		builtin_ipam.Init,
		remote_ipam.Init,
	} {
		if err := fn(ic, ds); err != nil {
			return err
		}
	}
	return nil
}
