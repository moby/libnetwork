package overlay

import "github.com/docker/libnetwork/netlabel"

const (
	// VxlanPortLabel Overlay network vxlan interface port label
	VxlanPortLabel = netlabel.DriverPrefix + ".overlay.vxlan.port"
)
