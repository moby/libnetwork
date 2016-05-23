package macvlan

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/libnetwork/datastore"
)

type datastoreConfig struct {
	ID               string
	Internal         bool
	MacvlanMode      string
	CreatedSlaveLink bool
	ParentFile       string
	ParentList       []string
	SubnetIPv6       []*ipv6Subnet
	Subnet4          []*ipv4Subnet
}

type configFile struct {
	Macvlan macvlanConfig
}

type macvlanConfig struct {
	Version string `toml:"version"`
	Parent  string `toml:"parent"`
}

func (d *driver) networkStore(nid string) *network {
	d.Lock()
	networks := d.networks
	d.Unlock()
	n, ok := networks[nid]
	if !ok {
		n = d.getNetworkFromStore(nid)
		if n != nil {
			n.driver = d
			n.endpoints = endpointTable{}
			n.once = &sync.Once{}
			networks[nid] = n
		}
	}
	if n == nil {
		return nil
	}

	return n
}

func (d *driver) getNetworkFromStore(nid string) *network {
	if d.store == nil {
		return nil
	}
	n := &network{id: nid}
	if err := d.store.GetObject(datastore.Key(n.Key()...), n); err != nil {
		return nil
	}

	return n
}

func (n *network) Key() []string {
	return []string{macvlanPrefix, "network", n.id}
}

func (n *network) KeyPrefix() []string {
	return []string{macvlanPrefix, "network"}
}

// SetValue encodes values to the datastore
func (n *network) Value() []byte {
	netJSON := []*datastoreConfig{}
	var v4Nets []*ipv4Subnet
	if len(n.config.Ipv4Subnets) > 0 {
		for _, v4 := range n.config.Ipv4Subnets {
			sub4 := &ipv4Subnet{
				SubnetIP: v4.SubnetIP,
				GwIP:     v4.GwIP,
			}
			v4Nets = append(v4Nets, sub4)
		}
	}
	var v6Nets []*ipv6Subnet
	if len(n.config.Ipv6Subnets) > 0 {
		for _, v6 := range n.config.Ipv6Subnets {
			sub6 := &ipv6Subnet{
				SubnetIP: v6.SubnetIP,
				GwIP:     v6.GwIP,
			}
			v6Nets = append(v6Nets, sub6)
		}
	}
	sj := &datastoreConfig{
		ID:               n.config.ID,
		Subnet4:          v4Nets,
		SubnetIPv6:       v6Nets,
		Internal:         n.config.Internal,
		ParentList:       n.config.ParentList,
		ParentFile:       n.config.ParentFile,
		MacvlanMode:      n.config.MacvlanMode,
		CreatedSlaveLink: n.config.CreatedSlaveLink,
	}
	netJSON = append(netJSON, sj)
	b, err := json.Marshal(netJSON)
	if err != nil {
		return []byte{}
	}

	return b
}

// SetValue decodes values from the datastore
func (n *network) SetValue(value []byte) error {
	dsConfig := []*datastoreConfig{}
	err := json.Unmarshal(value, &dsConfig)
	if err != nil {
		return err
	}
	for _, c := range dsConfig {
		n.config = &configuration{}
		n.config.ID = c.ID
		if len(c.Subnet4) > 0 {
			for _, c := range c.Subnet4 {
				v4 := &ipv4Subnet{
					SubnetIP: c.SubnetIP,
					GwIP:     c.GwIP,
				}
				n.config.Ipv4Subnets = append(n.config.Ipv4Subnets, v4)
			}
		}
		if len(c.SubnetIPv6) > 0 {
			for _, c := range c.SubnetIPv6 {
				v6 := &ipv6Subnet{
					SubnetIP: c.SubnetIP,
					GwIP:     c.GwIP,
				}
				n.config.Ipv6Subnets = append(n.config.Ipv6Subnets, v6)
			}
		}
		if len(c.ParentList) > 0 {
			n.config.Parent, err = getParent(c.ParentList)
			if err != nil {
				// throw error for parent t-shooting
				logrus.Errorf(err.Error())
				return err
			}
		}
		// parent file overwrites -o parent
		if c.ParentFile != "" {
			n.config.Parent, err = getParentFile(c.ParentFile)
			if err != nil {
				// Returned errors are suppressed, throw one for parent t-shooting
				logrus.Errorf(err.Error())
				return fmt.Errorf("a parent file was specified but encountered an error: %v", err)
			}
		}
		if c.Internal {
			n.config.Parent = getDummyName(stringid.TruncateID(n.id))
			if err := createDummyLink(n.config.Parent); err != nil {
				logrus.Errorf(err.Error())
				return err
			}
			n.config.Internal = true
		}
		n.config.MacvlanMode = c.MacvlanMode
		n.config.CreatedSlaveLink = c.CreatedSlaveLink
	}
	if !parentExists(n.config.Parent) {
		// if the --internal flag is set, create a dummy link
		if n.config.Internal {
			if err := createDummyLink(n.config.Parent); err != nil {
				return err
			}
			n.config.CreatedSlaveLink = true
		} else {
			// if the subinterface parent_iface.vlan_id checks do not pass, return err.
			//  a valid example is 'eth0.10' for a parent iface 'eth0' with a vlan id '10'
			if err := createVlanLink(n.config.Parent); err != nil {
				// throw error for parent troubleshooting
				logrus.Errorf(err.Error())
				return err
			}
			// if driver created the networks slave link, record it for future deletion
			n.config.CreatedSlaveLink = true
		}
	}

	return nil
}

func (n *network) Index() uint64 {
	return n.dbIndex
}

func (n *network) SetIndex(index uint64) {
	n.dbIndex = index
	n.dbExists = true
}

func (n *network) Exists() bool {
	return n.dbExists
}

func (n *network) Skip() bool {
	return false
}

func (n *network) writeToStore() error {
	if n.driver.store == nil {
		return nil
	}

	return n.driver.store.PutObjectAtomic(n)
}

func (n *network) DataScope() string {
	return datastore.GlobalScope
}
