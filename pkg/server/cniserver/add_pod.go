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
	"github.com/docker/libnetwork/pkg/cniapi"
)

func addPod(w http.ResponseWriter, r *http.Request, vars map[string]string) (_ interface{}, retErr error) {
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
	sbID, err := createSandbox(cniInfo.ContainerID, cniInfo.NetNS)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox for %q: %v", cniInfo.ContainerID, err)
	}
	defer func() {
		if retErr != nil {
			if err := deleteSandbox(sbID); err != nil {
				log.Warnf("failed to delete sandbox %v on setup pod failure , error:%v", sbID, err)
			}
		}
	}()

	ep, err := createEndpoint(cniInfo.ContainerID, cniInfo.NetConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create endpoint for %q: %v", cniInfo.ContainerID, err)
	}
	defer func() {
		if retErr != nil {
			if err := deleteEndpoint(ep.ID); err != nil {
				log.Warnf("failed to delete endpoint %v on setup pod failure , error:%v", ep.ID, err)
			}
		}
	}()

	if err = endpointJoin(sbID, ep.ID, cniInfo.NetNS); err != nil {
		return nil, fmt.Errorf("failed to attach endpoint to sandbox for container:%q,sandbox:%q,endpoint:%q, error:%v", cniInfo.ContainerID, sbID, ep.ID, err)
	}
	defer func() {
		if retErr != nil {
			if err = endpointLeave(sbID, ep.ID); err != nil {
				log.Warnf("failed to detach endpoint %q from sandbox %q , err:%v", ep.ID, sbID, err)
			}
		}
	}()

	cniService.endpointIDStore[cniInfo.ContainerID] = ep.ID
	cniService.sandboxIDStore[cniInfo.ContainerID] = sbID

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

func createSandbox(ContainerID, netns string) (string, error) {
	sc := client.SandboxCreate{ContainerID: ContainerID, UseExternalKey: true}
	obj, _, err := readBody(httpCall("POST", "/sandboxes", sc, nil))
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

func createEndpoint(ContainerID string, netConfig types.NetConf) (client.EndpointInfo, error) {
	var ep client.EndpointInfo
	sc := client.ServiceCreate{Name: ContainerID, Network: netConfig.Name, DisableResolution: true}
	obj, _, err := readBody(httpCall("POST", "/services", sc, nil))
	if err != nil {
		return ep, err
	}
	log.Errorf("createEndpoint result:%+v\n", ep)
	err = json.Unmarshal(obj, &ep)
	return ep, err
}

func endpointJoin(sandboxID, endpointID, netns string) (retErr error) {
	nc := client.ServiceAttach{SandboxID: sandboxID, SandboxKey: netns}

	_, _, err := readBody(httpCall("POST", "/services/"+endpointID+"/backend", nc, nil))

	return err
}
