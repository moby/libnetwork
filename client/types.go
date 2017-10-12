package client

import (
	"net"

	"github.com/docker/libnetwork/types"
)

/***********
 Resources
************/

// NetworkResource is the body of the "get network" http response message
type NetworkResource struct {
	Name     string             `json:"name"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Services []*serviceResource `json:"services"`
}

// serviceResource is the body of the "get service" http response message
type serviceResource struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	Network string `json:"network"`
}

// SandboxResource is the body of "get service backend" response message
type SandboxResource struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	ContainerID string `json:"container_id"`
}

/***********
  Body types
  ************/

// IPAMConf is the ipam configution used during network create
type IPAMConf struct {
	PreferredPool string
	SubPool       string
	Gateway       string
	AuxAddresses  map[string]string
}

// NetworkCreate is the expected body of the "create network" http request message
type NetworkCreate struct {
	Name        string            `json:"name"`
	ID          string            `json:"id"`
	NetworkType string            `json:"network_type"`
	IPv4Conf    []IPAMConf        `json:"ipv4_configuration"`
	DriverOpts  map[string]string `json:"driver_opts"`
	NetworkOpts map[string]string `json:"network_opts"`
}

// ServiceCreate represents the body of the "publish service" http request message
type ServiceCreate struct {
	Name              string   `json:"name"`
	MyAliases         []string `json:"my_aliases"`
	Network           string   `json:"network_name"`
	DisableResolution bool     `json:"disable_resolution"`
}

// ServiceDelete represents the body of the "unpublish service" http request message
type ServiceDelete struct {
	Name  string `json:"name"`
	Force bool   `json:"force"`
}

// ServiceAttach represents the expected body of the "attach/detach sandbox to/from service" http request messages
type ServiceAttach struct {
	SandboxID  string   `json:"sandbox_id"`
	Aliases    []string `json:"aliases"`
	SandboxKey string   `json:"sandbox_key"`
}

// SandboxCreate is the body of the "post /sandboxes" http request message
type SandboxCreate struct {
	ContainerID       string                `json:"container_id"`
	HostName          string                `json:"host_name"`
	DomainName        string                `json:"domain_name"`
	HostsPath         string                `json:"hosts_path"`
	ResolvConfPath    string                `json:"resolv_conf_path"`
	DNS               []string              `json:"dns"`
	ExtraHosts        []extraHost           `json:"extra_hosts"`
	UseExternalKey    bool                  `json:"use_external_key"`
	UseDefaultSandbox bool                  `json:"use_default_sandbox"`
	ExposedPorts      []types.TransportPort `json:"exposed_ports"`
	PortMapping       []types.PortBinding   `json:"port_mapping"`
}

// extraHost represents the extra host object
type extraHost struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// sandboxParentUpdate is the object carrying the information about the
// sandbox parent that needs to be updated.
type sandboxParentUpdate struct {
	ContainerID string `json:"container_id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
}

// EndpointInfo contants the endpoint info for http response message on endpoint creation
type EndpointInfo struct {
	ID          string           `json:"id"`
	Address     net.IPNet        `json:"address"`
	AddressIPv6 net.IPNet        `json:"address_ipv6"`
	MacAddress  net.HardwareAddr `json:"mac_address"`
	Gateway     net.IP           `json:"gateway"`
	GatewayIPv6 net.IP           `json:"gateway_ipv6"`
}
