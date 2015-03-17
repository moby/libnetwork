package libnetwork

import "fmt"

var drivers = map[string]DriverCreator{}

// DriverCreator returns a new Driver instance.
type DriverCreator func() (Driver, error)

// RegisterNetworkDriver associates a textual identifier with a way to create a
// new driver. It is called by the various network implementations, and used
// upon invokation of the libnetwork.NetNetwork function.
//
// For example:
//
//    type driver struct{}
//
//    func CreateDriver() (Driver, error) {
//        return &driver{}, nil
//    }
//
//    func init() {
//        RegisterNetworkDriver("test", CreateDriver)
//    }
//
func RegisterNetworkDriver(name string, creatorFn DriverCreator) error {
	// Store the new driver information to invoke at creation time.
	if _, ok := drivers[name]; ok {
		return fmt.Errorf("a driver for network type %q is already registed", name)
	}

	drivers[name] = creatorFn
	return nil
}

func createNetworkDriver(networkType string) (Driver, error) {
	if d, ok := drivers[networkType]; ok {
		return d()
	}
	return nil, fmt.Errorf("unknown driver %q", networkType)
}
