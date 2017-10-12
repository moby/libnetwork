package main

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	"github.com/sirupsen/logrus"

	cniserver "github.com/docker/libnetwork/provider/cni/server"
)

var (
	cniserverFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "sock",
			Value: "/var/run/cniserver.sock",
			Usage: "path to the socket file on which cniserver listens. Default (/var/run/cni-libnetwork.sock)",
		},
		cli.StringFlag{
			Name:  "dnet-port",
			Value: "2389",
			Usage: "Daemon socket to connect to. Default(2389)",
		},
		cli.StringFlag{
			Name:  "dnet-address",
			Value: "127.0.0.1",
			Usage: "Daemon IP address to connect to",
		},
		cli.BoolFlag{
			Name:  "D, -debug",
			Usage: "Enable debug mode",
		},
	}
)

func processFlags(c *cli.Context) error {
	var err error

	if c.Bool("D") {
		logrus.SetLevel(logrus.DebugLevel)
	}

	cniService, err := cniserver.NewCniService(c.String("sock"), c.String("dnet-address"), c.String("dnet-port"))
	if err != nil {
		return fmt.Errorf("failed to create cni service: %v", err)
	}
	serverCloseChan := make(chan struct{})
	if err := cniService.Init(serverCloseChan); err != nil {
		logrus.Errorf("Failed to initialize CNI server: %v", err)
		os.Exit(1)
	}
	// Wait on till the server closes
	<-serverCloseChan
	return nil
}
