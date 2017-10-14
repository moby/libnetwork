package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/sirupsen/logrus"
)

// Expects 3 args { [0] = "libnetwork-setkey", [1] = <container-id>, [2] = <controller-id> }
// It also expects configs.HookState as a json string in <stdin>
// Refer to https://github.com/opencontainers/runc/pull/160/ for more information
func main() {
	var err error

	// Return a failure to the calling process via ExitCode
	defer func() {
		if err != nil {
			logrus.Fatalf("%v", err)
		}
	}()

	// expecting 3 args {[0]="libnetwork-setkey", [1]=<container-id>, [2]=<controller-id> }
	if len(os.Args) < 3 {
		err = fmt.Errorf("expected 3 args, received : %d", len(os.Args))
		return
	}
	containerID := os.Args[1]

	// We expect configs.HookState as a json string in <stdin>
	stateBuf, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return
	}
	var state configs.HookState
	if err = json.Unmarshal(stateBuf, &state); err != nil {
		return
	}

	controllerID := os.Args[2]

	err = SetExternalKey(controllerID, containerID, fmt.Sprintf("/proc/%d/ns/net", state.Pid))
}
