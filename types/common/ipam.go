package common

import (
	"encoding/json"
	"net"

	"github.com/docker/libnetwork/driverapi"
	"github.com/docker/libnetwork/types"
)

// IpamConf contains all the ipam related configurations for a network
type IpamConf struct {
	// The master address pool for containers and network interfaces
	PreferredPool string
	// A subset of the master pool. If specified,
	// this becomes the container pool
	SubPool string
	// Preferred Network Gateway address (optional)
	Gateway string
	// Auxiliary addresses for network driver. Must be within the master pool.
	// libnetwork will reserve them if they fall into the container pool
	AuxAddresses map[string]string
}

// Validate checks whether the configuration is valid
func (c *IpamConf) Validate() error {
	if c.Gateway != "" && nil == net.ParseIP(c.Gateway) {
		return types.BadRequestErrorf("invalid gateway address %s in Ipam configuration", c.Gateway)
	}
	return nil
}

// IpamInfo contains all the ipam related operational info for a network
type IpamInfo struct {
	PoolID string
	Meta   map[string]string
	driverapi.IPAMData
}

// MarshalJSON encodes IpamInfo into json message
func (i *IpamInfo) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{
		"PoolID": i.PoolID,
	}
	v, err := json.Marshal(&i.IPAMData)
	if err != nil {
		return nil, err
	}
	m["IPAMData"] = string(v)

	if i.Meta != nil {
		m["Meta"] = i.Meta
	}
	return json.Marshal(m)
}

// UnmarshalJSON decodes json message into PoolData
func (i *IpamInfo) UnmarshalJSON(data []byte) error {
	var (
		m   map[string]interface{}
		err error
	)
	if err = json.Unmarshal(data, &m); err != nil {
		return err
	}
	i.PoolID = m["PoolID"].(string)
	if v, ok := m["Meta"]; ok {
		b, _ := json.Marshal(v)
		if err = json.Unmarshal(b, &i.Meta); err != nil {
			return err
		}
	}
	if v, ok := m["IPAMData"]; ok {
		if err = json.Unmarshal([]byte(v.(string)), &i.IPAMData); err != nil {
			return err
		}
	}
	return nil
}

// CopyTo deep copies to the destination IpamConfig
func (c *IpamConf) CopyTo(dstC *IpamConf) error {
	dstC.PreferredPool = c.PreferredPool
	dstC.SubPool = c.SubPool
	dstC.Gateway = c.Gateway
	if c.AuxAddresses != nil {
		dstC.AuxAddresses = make(map[string]string, len(c.AuxAddresses))
		for k, v := range c.AuxAddresses {
			dstC.AuxAddresses[k] = v
		}
	}
	return nil
}

// CopyTo deep copies to the destination IpamInfo
func (i *IpamInfo) CopyTo(dstI *IpamInfo) error {
	dstI.PoolID = i.PoolID
	if i.Meta != nil {
		dstI.Meta = make(map[string]string)
		for k, v := range i.Meta {
			dstI.Meta[k] = v
		}
	}

	dstI.AddressSpace = i.AddressSpace
	dstI.Pool = types.GetIPNetCopy(i.Pool)
	dstI.Gateway = types.GetIPNetCopy(i.Gateway)

	if i.AuxAddresses != nil {
		dstI.AuxAddresses = make(map[string]*net.IPNet)
		for k, v := range i.AuxAddresses {
			dstI.AuxAddresses[k] = types.GetIPNetCopy(v)
		}
	}

	return nil
}
