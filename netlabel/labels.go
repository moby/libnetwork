package netlabel

const (
	// GenericData constant that helps to identify an option as a Generic constant
	GenericData = "io.docker.network.generic"

	// GenericLabels constant that helps to identify an option as a Generic list of labels
	GenericLabels = "io.docker.network.genericlabels"

	// PortMap constant represents Port Mapping
	PortMap = "io.docker.network.endpoint.portmap"

	// MacAddress constant represents Mac Address config of a Container
	MacAddress = "io.docker.network.endpoint.macaddress"

	// ExposedPorts constant represents the container's Exposed Ports
	ExposedPorts = "io.docker.network.endpoint.exposedports"

	// PortBinding constant represents the container to host Port Binding
	PortBinding = "io.docker.network.endpoint.portbinding"

	// EnableIPv6 constant represents enabling IPV6 at network level
	EnableIPv6 = "io.docker.network.enable_ipv6"

	// NetworkIPv4 constant represents the ipv4 network
	NetworkIPv4 = "io.docker.network.networkipv4"

	// NetworkIPv6 constant represents the ipv6 network
	NetworkIPv6 = "io.docker.network.networkipv6"

	// ContainersSubnetIPv4 constant represents the IPv4 subnet being assigned to the containers
	ContainersSubnetIPv4 = "io.docker.network.containersubnet"

	// ContainersSubnetIPv6 constant represents the IPv6 subnet being assigned to the containers
	ContainersSubnetIPv6 = "io.docker.network.containersubnetv6"

	// DefaultGatewayIPv4 constant represents the IPv4 gateway being assigned to the containers
	DefaultGatewayIPv4 = "io.docker.network.defaultgatewayv4"

	// DefaultGatewayIPv6 constant represents the IPv6 gateway being assigned to the containers
	DefaultGatewayIPv6 = "io.docker.network.defaultgatewayv6"

	// HostName constant represents the Hostname to be configured on the container
	HostName = "io.docker.network.hostname"

	// DomainName constant represents the Hostname to be configured on the container
	DomainName = "io.docker.network.domainname"

	// HostsPath constant represents the Host Path to be configured on the container
	HostsPath = "io.docker.network.hostpath"

	// ResolvConfPath constant represents the resolv.conf path to be configured on the container
	ResolvConfPath = "io.docker.network.resolvconf"

	// DNS constant represents the DNS to be configured on the container
	DNS = "io.docker.network.dns"

	// ExtraHost constant represents the ExtraHosts to be configured on the container
	ExtraHost = "io.docker.network.extrahost"

	// ParentUpdate constant represents the ParentUpdate to be configured on the endpoint
	ParentUpdate = "io.docker.endpoint.parentupdate"

	// UseDefaultSandbox constant represents the request to use or not the default sandbox
	UseDefaultSandbox = "io.docker.endpoint.usedefaultsandbox"
)
