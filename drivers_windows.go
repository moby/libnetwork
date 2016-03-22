package libnetwork

import (
	"github.com/docker/libnetwork/drivers/null"
	"github.com/docker/libnetwork/drivers/windows"
)

func (c *controller) getInitializers() error {
	in := map[string]initializer{
		"null":        null.Init,
		"transparent": windows.GetInit("transparent"),
		"l2bridge":    windows.GetInit("l2bridge"),
		"l2tunnel":    windows.GetInit("l2tunnel"),
		"nat":         windows.GetInit("nat"),
	}
	c.Lock()
	c.initializers = in
	c.Unlock()
	return nil
}
