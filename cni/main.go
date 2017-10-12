package main

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/docker/libnetwork/pkg/cniapi"
	log "github.com/sirupsen/logrus"
)

func cmdAdd(args *skel.CmdArgs) error {
	c := cniapi.NewDnetCniClient()
	result, err := c.SetupPod(args)
	if err != nil {
		return fmt.Errorf("failed to setup Pod: %v", err)
	}
	return types.PrintResult(result, version.Current())
}

func cmdDel(args *skel.CmdArgs) error {
	c := cniapi.NewDnetCniClient()
	if err := c.TearDownPod(args); err != nil {
		return fmt.Errorf("failed to tear down pod: %v", err)
	}
	return nil
}

func main() {
	log.Infof("Dnet CNI plugin")
	skel.PluginMain(cmdAdd, cmdDel, version.PluginSupports("", "0.1.0", "0.2.0", version.Current()))
}
