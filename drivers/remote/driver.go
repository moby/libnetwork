package remote

import (
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libnetwork/sandbox"
	"github.com/docker/libnetwork/types"
)

// We duplicate the structs from driverapi as
// json doesn't like encoding and decoding net.IPAddr
type iface struct {
	SrcName string
	DstName string
	Address string
}

type sbInfo struct {
	Interfaces  []*iface
	Gateway     net.IP
	GatewayIPv6 net.IP
}

// Remote represents an external process to be used
// as an external network driver.  Trasport details
// (HTTP over a TCP or Unix socker, for instance) are
// left to the implementor.
type Remote interface {
	Call(method, path string, data interface{}) (io.ReadCloser, error)
}

// Driver is a network Driver for external plugins.
type Driver struct {
	remote Remote
}

func (sb *sbInfo) toSandboxInfo() (*sandbox.Info, error) {
	ifaces := make([]*sandbox.Interface, len(sb.Interfaces))
	for i, inIf := range sb.Interfaces {
		outIf := &sandbox.Interface{
			SrcName: inIf.SrcName,
			DstName: inIf.DstName,
		}
		ip, ipnet, err := net.ParseCIDR(inIf.Address)
		if err != nil {
			return nil, err
		}
		ipnet.IP = ip
		outIf.Address = ipnet
		ifaces[i] = outIf
	}
	return &sandbox.Info{
		Interfaces:  ifaces,
		Gateway:     nil,
		GatewayIPv6: nil,
	}, nil
}

// New create a new Driver given the plugin.
func New(remote Remote) *Driver {
	return &Driver{remote}
}

// Type returns the the type of this driver, the network type this driver manages
func (driver *Driver) Type() string {
	return "remote"
}

// Config pushes driver specific config to the driver
func (driver *Driver) Config(config interface{}) error {
	reader, err := driver.remote.Call("POST", "config", config)
	if err != nil {
		logrus.Warningf("Driver returned err:", err)
		return err
	}
	reader.Close()
	return nil
}

// CreateNetwork invokes the driver method to create a network passing
// the network id and network specific config. The config mechanism will
// eventually be replaced with labels which are yet to be introduced.
func (driver *Driver) CreateNetwork(nid types.UUID, config interface{}) error {
	reader, err := driver.remote.Call("PUT", string(nid), config)
	if err != nil {
		logrus.Warningf("Driver returned err:", err)
		return err
	}
	reader.Close()
	return nil
}

// DeleteNetwork invokes the driver method to delete network passing
// the network id.
func (driver *Driver) DeleteNetwork(nid types.UUID) error {
	reader, err := driver.remote.Call("DELETE", string(nid), nil)
	if err != nil {
		logrus.Warningf("Driver returned err:", err)
		return err
	}
	reader.Close()
	return nil
}

// CreateEndpoint invokes the driver method to create an endpoint
// passing the network id, endpoint id, sandbox key and driver
// specific config. The config mechanism will eventually be replaced
// with labels which are yet to be introduced.
func (driver *Driver) CreateEndpoint(nid, eid types.UUID, key string, config interface{}) (*sandbox.Info, error) {
	reader, err := driver.remote.Call("PUT", fmt.Sprintf("%s/%s", nid, eid), config)
	if err != nil {
		logrus.Warningf("Driver returned err:", err)
		return nil, err
	}
	defer reader.Close()
	var sbinfo sbInfo
	if err := json.NewDecoder(reader).Decode(&sbinfo); err != nil {
		logrus.Warningf("Driver returned invalid JSON:", err)
		return nil, err
	}

	var sbInfo *sandbox.Info
	if sbInfo, err = sbinfo.toSandboxInfo(); err != nil {
		logrus.Warningf("Unable to convert sbInfo")
		return nil, err
	}
	logrus.Infof("Plugin returned %+v", sbinfo)
	return sbInfo, nil
}

// DeleteEndpoint invokes the driver method to delete an endpoint
// passing the network id and endpoint id.
func (driver *Driver) DeleteEndpoint(nid, eid types.UUID) error {
	path := fmt.Sprintf("%s/%s", nid, eid)
	reader, err := driver.remote.Call("DELETE", path, nil)
	if err != nil {
		logrus.Warningf("Driver returned err:", err)
		return err
	}
	reader.Close()
	return nil
}
