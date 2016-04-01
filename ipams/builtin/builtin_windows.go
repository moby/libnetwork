// +build windows

package builtin

import (
	"github.com/docker/libnetwork/ipamapi"

	windowsipam "github.com/docker/libnetwork/ipams/windowsipam"
)

// Init registers the built-in ipam service with libnetwork
func Init(ic ipamapi.Callback, config map[string]interface{}) error {
	initFunc := windowsipam.GetInit(ipamapi.DefaultIPAM)

	return initFunc(ic, config)
}
