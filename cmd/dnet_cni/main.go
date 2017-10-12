package main

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/version"
	log "github.com/sirupsen/logrus"

	"github.com/docker/libnetwork/provider/cni/cniapi"
)

func cmdAdd(args *skel.CmdArgs) error {
	c := cniapi.NewDnetCniClient()
	result, err := c.AddWorkload(args)
	if err != nil {
		return fmt.Errorf("failed to add workload: %v", err)
	}
	return types.PrintResult(result, version.Current())
}

func cmdDel(args *skel.CmdArgs) error {
	c := cniapi.NewDnetCniClient()
	if err := c.DeleteWorkload(args); err != nil {
		return fmt.Errorf("failed to delete workload: %v", err)
	}
	return nil
}

func main() {
	log.Infof("Dnet CNI plugin")
	skel.PluginMain(cmdAdd, cmdDel, version.PluginSupports("", "0.1.0", "0.2.0", version.Current()))
}
