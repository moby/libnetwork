package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"reflect"

	"github.com/containernetworking/cni/pkg/types/current"
	log "github.com/sirupsen/logrus"

	"github.com/docker/libnetwork/provider/cni"
	"github.com/docker/libnetwork/provider/cni/cniapi"
)

func addWorkload(w http.ResponseWriter, r *http.Request, c cni.Service, vars map[string]string) (_ interface{}, retErr error) {
	cniInfo := cniapi.CniInfo{}
	var result current.Result

	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read request: %v", err)
		return cniInfo, err
	}

	if err := json.Unmarshal(content, &cniInfo); err != nil {
		return cniInfo, err
	}

	log.Infof("Received add workload request %+v", cniInfo)
	ep, err := c.SetupWorkload(cniInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to setup workload: %v", err)
	}
	result.Interfaces = append(result.Interfaces, &current.Interface{Name: "eth1", Mac: ep.MacAddress.String()})
	if !reflect.DeepEqual(ep.Address, (net.IPNet{})) {
		result.IPs = append(result.IPs, &current.IPConfig{
			Version: "4",
			Address: ep.Address,
			Gateway: ep.Gateway,
		})
	}
	if !reflect.DeepEqual(ep.AddressIPv6, (net.IPNet{})) {
		result.IPs = append(result.IPs, &current.IPConfig{
			Version: "6",
			Address: ep.AddressIPv6,
			Gateway: ep.GatewayIPv6,
		})
	}
	//TODO : Point IPs to the interface index

	return result, err

}
