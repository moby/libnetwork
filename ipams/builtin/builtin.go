package builtin

import (
	"fmt"

	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/ipam"
	"github.com/docker/libnetwork/ipamapi"
)

// Init registers the built-in ipam service with libnetwork
func Init(ic ipamapi.Callback, n interface{}) error {
	var (
		ok bool
		ds datastore.DataStore
	)

	if n != nil {
		if ds, ok = n.(datastore.DataStore); !ok {
			return fmt.Errorf("incorrect datastore passed to built-in ipam init")
		}
	}

	a, err := ipam.NewAllocator(ds)
	if err != nil {
		return err
	}

	return ic.RegisterIpam("builtin-ipam", a, a)
}
