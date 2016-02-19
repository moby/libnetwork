// +build linux freebsd

package builtin

import (
	"github.com/docker/libnetwork/ipam"
	"github.com/docker/libnetwork/ipamapi"
)

// Init registers the built-in ipam service with libnetwork
func Init(ic ipamapi.Callback, config map[string]interface{}) error {
	a, err := ipam.NewAllocator(config)
	if err != nil {
		return err
	}

	return ic.RegisterIpamDriver(ipamapi.DefaultIPAM, a)
}
