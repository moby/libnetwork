package bridge

const (
	// BridgeName label for bridge driver
	BridgeName = "io.docker.network.bridge.name"

	// EnableIPTables label for bridge driver
	EnableIPTables = "io.docker.network.bridge.enable_iptables"

	// EnableIPMasquerade label for bridge driver
	EnableIPMasquerade = "io.docker.network.bridge.enableipmasquerade"

	// EnableUserlandProxy label
	EnableUserlandProxy = "io.docker.network.bridge.enable_userland_proxy"

	// EnableICC label
	EnableICC = "io.docker.network.bridge.enable_icc"

	// MTU label
	MTU = "io.docker.network.bridge.mtu"

	// DefaultBindingIP label
	DefaultBindingIP = "io.docker.network.bridge.host_binding_ipv4"

	// AllowNonDefaultBridge label
	AllowNonDefaultBridge = "io.docker.network.bridge.allow_non_default_bridge"
)
