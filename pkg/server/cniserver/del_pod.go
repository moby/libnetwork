package server

import (
	"encoding/json"
	"fmt"
	"github.com/docker/libnetwork/client"
	"github.com/docker/libnetwork/pkg/cniapi"
	"io/ioutil"
	"net/http"
)

func deletePod(w http.ResponseWriter, r *http.Request, vars map[string]string) (interface{},error) {
	//TODO: need to explore force cleanup and test for parallel delete pods
	cniInfo := cniapi.CniInfo{}

	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request: %v", err)
	}

	if err = json.Unmarshal(content, &cniInfo); err != nil {
		return nil, err
	}
	fmt.Printf("Received delete pod request %+v", cniInfo)

	//Gather sandboxID and the endpointID
	sbID, err := lookupSandboxID(cniInfo.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("sandbox lookup failed for containerID %q , error:%v", cniInfo.ContainerID, err)
	}
	epID, err := lookupEndpointID(cniInfo.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("endpoint lookup failed for containerID %q, error:%v", cniInfo.ContainerID, err)
	}

	if err = endpointLeave(sbID, epID); err != nil {
		return nil, fmt.Errorf("failed to leave endpoint from sandbox for container:%q,sandbox:%q,endpoint:%q, error:%v", cniInfo.ContainerID, sbID, epID, err)
	}

	if err = deleteEndpoint(epID); err != nil {
		return nil, fmt.Errorf("failed to delete endpoint %q for container %q,, error:%v",
			epID, cniInfo.ContainerID, err)
	}

	if err = deleteSandbox(sbID); err != nil {
		return nil, fmt.Errorf("failed to delete sandbox %q for container %q, error:%v", sbID, cniInfo.ContainerID, err)
	}
	delete(cniService.endpointIDStore, epID)
	delete(cniService.sandboxIDStore, sbID)
	return nil, nil
}

func endpointLeave(sandboxID, endpointID string) error {
	_, _, err := readBody(httpCall("DELETE", "/services/"+endpointID+"/backend/"+sandboxID, nil, nil))
	return err
}

func deleteSandbox(sandboxID string) error {
	_, _, err := readBody(httpCall("DELETE", "/sandboxes/"+sandboxID, nil, nil))
	return err
}

func deleteEndpoint(endpointID string) error {
	sd := client.ServiceDelete{Name: endpointID, Force: true}
	_, _, err := readBody(httpCall("DELETE", "/services/"+endpointID, sd, nil))
	return err
}

func lookupSandboxID(containerID string) (string, error) {
	if id, ok := cniService.sandboxIDStore[containerID]; ok {
		return id, nil
	}
	obj, _, err := readBody(httpCall("GET", fmt.Sprintf("/sandboxes?partial-container-id=%s", containerID), nil, nil))
	if err != nil {
		return "", err
	}

	var sandboxList []client.SandboxResource
	err = json.Unmarshal(obj, &sandboxList)
	if err != nil {
		return "", err
	}

	if len(sandboxList) == 0 {
		return "", fmt.Errorf("sandbox not found")
	}

	cniService.sandboxIDStore[containerID] = sandboxList[0].ID
	return sandboxList[0].ID, nil
}

func lookupEndpointID(containerID string) (string, error) {
	if id, ok := cniService.endpointIDStore[containerID]; ok {
		return id, nil
	}
	return "", fmt.Errorf("endpoint not found")
	//TODO: query libnetwork core if the cache doesnt have it.
}
