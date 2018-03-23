package main

import (
	"github.com/docker/libnetwork/cmd/test-image/exec"
	"github.com/docker/libnetwork/cmd/test-image/icmp"
	"github.com/docker/libnetwork/cmd/test-image/server"
	"github.com/docker/libnetwork/cmd/test-image/tcp"
)

func main() {
	s := server.NewTestServer()
	s.Init()
	// enable ICMP endpoints
	icmp.Init(s)
	tcp.Init(s)
	exec.Init(s)

	s.Start("0.0.0.0", 2000)
	select {}
}
