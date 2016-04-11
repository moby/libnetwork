package libnetwork

import (
	"github.com/docker/libnetwork/drivers/bridge"
	"github.com/docker/libnetwork/drivers/host"
	"github.com/docker/libnetwork/drivers/null"
	"github.com/docker/libnetwork/drivers/overlay"
	"github.com/docker/libnetwork/drivers/remote"
)

func (c *controller) getInitializers() error {
	in := map[string]initializer{
		"bridge":  bridge.Init,
		"host":    host.Init,
		"null":    null.Init,
		"overlay": overlay.Init,
	}
	for k, v := range additionalDrivers() {
		in[k] = v
	}
	c.Lock()
	c.initializers = in
	c.Unlock()
	if err := remote.Init(c, makeDriverConfig(c, "remote")); err != nil {
		return err
	}
	return nil
}
