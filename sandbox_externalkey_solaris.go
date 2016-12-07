// +build solaris

package libnetwork

import (
	"net"

	"github.com/docker/libnetwork/types"
)

// no-op on non linux systems
func (c *controller) startExternalKeyListener() error {
	return nil
}

func (c *controller) acceptClientConnections(sock string, l net.Listener) {
}

func (c *controller) processExternalKey(conn net.Conn) error {
	return types.NotImplementedErrorf("processExternalKey isn't supported on non linux systems")
}

func (c *controller) stopExternalKeyListener() {
}
