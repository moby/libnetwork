package libnetwork

import (
	windriver "github.com/docker/libnetwork/drivers/windows"
	"github.com/docker/libnetwork/options"
	"github.com/docker/libnetwork/types"
	"github.com/docker/libnetwork/types/common"
)

const libnGWNetwork = "nat"

func getPlatformOption() common.EndpointOption {

	epOption := options.Generic{
		windriver.DisableICC: true,
		windriver.DisableDNS: true,
	}
	return EndpointOptionGeneric(epOption)
}

func (c *controller) createGWNetwork() (common.Network, error) {
	return nil, types.NotImplementedErrorf("default gateway functionality is not implemented in windows")
}
