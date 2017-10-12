package server

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net"
	"net/http"
	"reflect"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/docker/libnetwork/client"
	"github.com/docker/libnetwork/netutils"
	"github.com/docker/libnetwork/pkg/cniapi"
)

func addPod(w http.ResponseWriter, r *http.Request, c *CniService, vars map[string]string) (_ interface{}, retErr error) {
	cniInfo := cniapi.CniInfo{}
	var result current.Result

	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read request: %v", err)
		return nil, err
	}

	if err := json.Unmarshal(content, &cniInfo); err != nil {
		return nil, err
	}

	log.Infof("Received add pod request %+v", cniInfo)
	// Create a Sandbox
	sbID, err := c.createSandbox(cniInfo.ContainerID, cniInfo.NetNS)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox for %q: %v", cniInfo.ContainerID, err)
	}
	defer func() {
		if retErr != nil {
			if err := c.deleteSandbox(sbID); err != nil {
				log.Warnf("failed to delete sandbox %v on setup pod failure , error:%v", sbID, err)
			}
		}
	}()
	// Create an Endpoint
	ep, err := c.createEndpoint(cniInfo.ContainerID, cniInfo.NetConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create endpoint for %q: %v", cniInfo.ContainerID, err)
	}
	defer func() {
		if retErr != nil {
			if err := c.deleteEndpoint(ep.ID); err != nil {
				log.Warnf("failed to delete endpoint %v on setup pod failure , error:%v", ep.ID, err)
			}
		}
	}()
	// Attach endpoint to the sandbox
	if err = c.endpointJoin(sbID, ep.ID, cniInfo.NetNS); err != nil {
		return nil, fmt.Errorf("failed to attach endpoint to sandbox for container:%q,sandbox:%q,endpoint:%q, error:%v", cniInfo.ContainerID, sbID, ep.ID, err)
	}
	defer func() {
		if retErr != nil {
			if err = c.endpointLeave(sbID, ep.ID); err != nil {
				log.Warnf("failed to detach endpoint %q from sandbox %q , err:%v", ep.ID, sbID, err)
			}
		}
	}()

	c.endpointIDStore[cniInfo.ContainerID] = ep.ID
	c.sandboxIDStore[cniInfo.ContainerID] = sbID

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
	//TODO (Abhi): Point IPs to the interface index

	return result, err

}

func (c *CniService) createSandbox(ContainerID, netns string) (string, error) {
	sc := client.SandboxCreate{ContainerID: ContainerID, UseExternalKey: true}
	obj, _, err := netutils.ReadBody(c.dnetConn.HttpCall("POST", "/sandboxes", sc, nil))
	if err != nil {
		return "", err
	}

	var replyID string
	err = json.Unmarshal(obj, &replyID)
	if err != nil {
		return "", err
	}
	return replyID, nil
}

func (c *CniService) createEndpoint(ContainerID string, netConfig types.NetConf) (client.EndpointInfo, error) {
	var ep client.EndpointInfo
	// Create network if it doesnt exist. Need to handle refcount to delete
	// network on last pod delete. Also handle different network types and option
	if !c.networkExists(netConfig.Name) {
		if err := c.createNetwork(netConfig.Name, "overlay"); err != nil {
			return ep, err
		}
	}

	sc := client.ServiceCreate{Name: ContainerID, Network: netConfig.Name, DisableResolution: true}
	obj, _, err := netutils.ReadBody(c.dnetConn.HttpCall("POST", "/services", sc, nil))
	if err != nil {
		return ep, err
	}
	log.Errorf("createEndpoint result:%+v\n", ep)
	err = json.Unmarshal(obj, &ep)
	return ep, err
}

func (c *CniService) endpointJoin(sandboxID, endpointID, netns string) (retErr error) {
	nc := client.ServiceAttach{SandboxID: sandboxID, SandboxKey: netns}
	_, _, err := netutils.ReadBody(c.dnetConn.HttpCall("POST", "/services/"+endpointID+"/backend", nc, nil))
	return err
}

func (c *CniService) networkExists(networkName string) bool {
	obj, statusCode, err := netutils.ReadBody(c.dnetConn.HttpCall("GET", "/networks?name="+networkName, nil, nil))
	if err != nil {
		fmt.Printf("%s network does not exists \n", networkName)
		return false
	}
	if statusCode != http.StatusOK {
		fmt.Printf("%s network does not exists \n", networkName)
		return false
	}
	var list []*client.NetworkResource
	err = json.Unmarshal(obj, &list)
	if err != nil {
		return false
	}
	fmt.Printf("%s network exists \n", networkName)
	return (len(list) != 0)
}

// createNetwork is a very simple utility to create a default network
// if not present. This needs to be expanded into a more full utility function
func (c *CniService) createNetwork(networkName string, driver string) error {
	fmt.Printf("Creating a network %s driver: %s \n", networkName, driver)
	driverOpts := make(map[string]string)
	driverOpts["hostaccess"] = ""
	nc := client.NetworkCreate{Name: networkName, NetworkType: driver,
		DriverOpts: driverOpts}
	obj, _, err := netutils.ReadBody(c.dnetConn.HttpCall("POST", "/networks", nc, nil))
	if err != nil {
		return err
	}
	var replyID string
	err = json.Unmarshal(obj, &replyID)
	if err != nil {
		return err
	}
	return nil
}
