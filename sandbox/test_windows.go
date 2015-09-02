package sandbox

import "testing"

// SetupTestOSContext sets up a separate test  OS context in which tests will be executed.
func SetupTestOSContext(t *testing.T) func() {
	return func() {}
}
