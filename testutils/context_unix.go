// +build linux freebsd

package testutils

import (
	"os"
	"runtime"
	"testing"

	"github.com/docker/libnetwork/ns"
	"golang.org/x/sys/unix"
)

// SetupTestOSContext joins a new network namespace, and returns its associated
// teardown function.
//
// Example usage:
//
//     defer SetupTestOSContext(t)()
//
func SetupTestOSContext(t *testing.T) func() {
	runtime.LockOSThread()
	if err := unix.Unshare(unix.CLONE_NEWNET); err != nil {
		t.Fatalf("Failed to enter netns: %v", err)
	}

	fd, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY, 0)
	if err != nil {
		t.Fatal("Failed to open netns file")
	}

	// Since we are switching to a new test namespace make
	// sure to re-initialize initNs context
	ns.Init()

	runtime.LockOSThread()

	return func() {
		if err := unix.Close(fd); err != nil {
			t.Logf("Warning: netns closing failed (%v)", err)
		}
		runtime.UnlockOSThread()
	}
}

// RunningOnCircleCI returns true if being executed on libnetwork Circle CI setup
func RunningOnCircleCI() bool {
	return os.Getenv("CIRCLECI") != ""
}
