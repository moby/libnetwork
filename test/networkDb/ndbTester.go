package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/docker/libnetwork/diagnose"
	"github.com/docker/libnetwork/networkdb"
)

var nDB *networkdb.NetworkDB
var server diagnose.Server
var localNodeName string

func main() {
	if len(os.Args) < 3 {
		log.Fatal("You need to specify node name and port number")
	}
	localNodeName = os.Args[1]
	port, _ := strconv.Atoi(os.Args[2])

	ip, err := getIPpInterface("eth0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s There was a problem with the IP %s\n", localNodeName, err)
		return
	}

	fmt.Printf("%s uses IP %s\n", localNodeName, ip)

	server = diagnose.Server{}
	server.Init()
	nDB, err = networkdb.New(&networkdb.Config{
		AdvertiseAddr: ip,
		BindAddr:      ip,
		NodeName:      localNodeName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s error in the DB init %s\n", localNodeName, err)
		return
	}

	// Register network db handlers
	server.RegisterHandler(nDB, networkdb.NetDbPaths2Func)
	server.EnableDebug("", port)
	time.Sleep(120 * time.Minute)
}

func getIPpInterface(name string) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Name != name {
			continue // not the name specified
		}

		if iface.Flags&net.FlagUp == 0 {
			return "", errors.New("Interfaces is down")
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			return ip.String(), nil
		}
		return "", errors.New("Interfaces does not have a valid IPv4")
	}
	return "", errors.New("Interface not found")
}
