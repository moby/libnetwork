package main

import (
	"fmt"
	"log"
	"net"

	"github.com/docker/libnetwork"
	"github.com/docker/libnetwork/drivers/bridge"
)

func main() {
	ip, net, _ := net.ParseCIDR("192.168.100.1/24")
	net.IP = ip

	drv, err := libnetwork.NewDriver("simplebridge")
	if err != nil {
		log.Fatal(err)
	}

	options := bridge.Configuration{AddressIPv4: net}
	netw, err := drv.NewNetwork("dummy", options)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Network=%#v\n", netw)
}
