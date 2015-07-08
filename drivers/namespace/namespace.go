package namespace

import (
	"fmt"
	"os"
	"sync"

	"github.com/docker/libnetwork/driverapi"
	"github.com/docker/libnetwork/netlabel"
	"github.com/docker/libnetwork/options"
	"github.com/docker/libnetwork/sandbox"
	"github.com/docker/libnetwork/types"
)

const networkType = "namespace"

type driver struct {
	network types.UUID
	sync.Mutex
}

// networkConfiguration for network specific configuration
type endpointConfig struct {
	ContainerID     string
	CustomNamespace string
}

// Init registers a new instance of namespace driver
func Init(dc driverapi.DriverCallback) error {
	c := driverapi.Capability{
		Scope: driverapi.LocalScope,
	}
	return dc.RegisterDriver(networkType, &driver{}, c)
}

func (d *driver) Config(option map[string]interface{}) error {
	return nil
}

func (d *driver) CreateNetwork(id types.UUID, option map[string]interface{}) error {
	d.Lock()
	defer d.Unlock()

	d.network = id
	return nil
}

func (d *driver) DeleteNetwork(nid types.UUID) error {
	return nil
}

func (d *driver) CreateEndpoint(nid, eid types.UUID, epInfo driverapi.EndpointInfo, epOptions map[string]interface{}) error {
	return nil
}

func (d *driver) DeleteEndpoint(nid, eid types.UUID) error {
	return nil
}

func (d *driver) EndpointOperInfo(nid, eid types.UUID) (map[string]interface{}, error) {
	return make(map[string]interface{}, 0), nil
}

// Join method is invoked when a Sandbox is attached to an endpoint.
func (d *driver) Join(nid, eid types.UUID, sboxKey string, jinfo driverapi.JoinInfo, options map[string]interface{}) error {
	//Symlink the namespace that the user provided to docker's namespace directory
	config, err := parseNamespaceOptions(options)
	if err != nil {
		return err
	}
	return symlinkNamespace(config)
}

// Leave method is invoked when a Sandbox detaches from an endpoint.
func (d *driver) Leave(nid, eid types.UUID) error {
	return nil
}

func (d *driver) Type() string {
	return networkType
}

//parseNamespaceOptions parses the generic options into an endpointConfig struct
//The location of the network namespace to use, as well as the
//Container ID are passed into the driver via generic options.
func parseNamespaceOptions(nOptions map[string]interface{}) (*endpointConfig, error) {
	if nOptions == nil {
		return nil, nil
	}
	genericData, ok := nOptions[netlabel.GenericNamespaceOptions]
	if ok && genericData != nil {
		switch opt := genericData.(type) {
		case options.Generic:
			opaqueConfig, err := options.GenerateFromModel(opt, &endpointConfig{})
			if err != nil {
				return nil, err
			}
			return opaqueConfig.(*endpointConfig), nil
		case *endpointConfig:
			return opt, nil
		default:
			return nil, fmt.Errorf("did not recognize options type: %v\n", opt)
		}
	}
	return nil, fmt.Errorf("nil or non-existent generic namespace opts in join options")
}

//Symlinks the container's network namespace file in /var/run/docker/netns/{id} to the
//custom namespace path provided
func symlinkNamespace(epConfig *endpointConfig) error {
	if epConfig == nil {
		return fmt.Errorf("cannot join namespace: nil namespace options provided")
	}

	if epConfig.CustomNamespace == "" {
		return fmt.Errorf("cannot join namespace: no namespace path provided")
	}

	if epConfig.ContainerID == "" {
		return fmt.Errorf("cannot join namespace: no container ID provided")
	}

	if _, err := os.Stat(epConfig.CustomNamespace); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot join namespace: %v", err)
		}
	}
	return os.Symlink(epConfig.CustomNamespace, sandbox.GenerateKey(epConfig.ContainerID))
}
