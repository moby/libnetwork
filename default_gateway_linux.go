package libnetwork

import (
	"fmt"
	"strconv"

	"github.com/docker/libnetwork/drivers/bridge"
	"github.com/docker/libnetwork/types/common"
)

const libnGWNetwork = "docker_gwbridge"

func getPlatformOption() common.EndpointOption {
	return nil
}

func (c *controller) createGWNetwork() (common.Network, error) {
	netOption := map[string]string{
		bridge.BridgeName:         libnGWNetwork,
		bridge.EnableICC:          strconv.FormatBool(false),
		bridge.EnableIPMasquerade: strconv.FormatBool(true),
	}

	n, err := c.NewNetwork("bridge", libnGWNetwork, "",
		NetworkOptionDriverOpts(netOption),
		NetworkOptionEnableIPv6(false),
	)

	if err != nil {
		return nil, fmt.Errorf("error creating external connectivity network: %v", err)
	}
	return n, err
}
