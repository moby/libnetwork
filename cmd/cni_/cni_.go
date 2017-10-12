package main

import (
	"io"
	"os"

	"github.com/codegangsta/cli"
	"github.com/docker/docker/pkg/reexec"
	"github.com/docker/docker/pkg/term"
	"github.com/sirupsen/logrus"
)

func main() {
	if reexec.Init() {
		return
	}
	_, stdout, stderr := term.StdStreams()
	logrus.SetOutput(stderr)
	err := cniApp(stdout, stderr)
	if err != nil {
		os.Exit(1)
	}
}

func cniApp(stdout, stderr io.Writer) error {
	app := cli.NewApp()

	app.Name = "cniserver"
	app.Usage = "A cni side car for libnetwork daemon."
	app.Flags = cniserverFlags
	app.Before = processFlags
	//app.Commands = cniCommands

	app.Run(os.Args)
	return nil
}
