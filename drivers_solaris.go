package libnetwork

import (
	"github.com/docker/libnetwork/drivers/null"
)

func getInitializers() []initializer {
	return []initializer{
		{null.Init, "null"},
	}
}
