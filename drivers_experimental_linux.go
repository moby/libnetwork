// +build experimental

package libnetwork

import (
	"github.com/docker/libnetwork/drivers/ipvlan"
	"github.com/docker/libnetwork/drivers/macvlan"
)

func additionalDrivers() map[string]initializer {
	return map[string]initializer{
		"macvlan": macvlan.Init,
		"ipvlan":  ipvlan.Init,
	}
}
