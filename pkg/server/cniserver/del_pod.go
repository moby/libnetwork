package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/docker/libnetwork/client"
	"github.com/docker/libnetwork/netutils"
	"github.com/docker/libnetwork/pkg/cniapi"
)

func deletePod(w http.ResponseWriter, r *http.Request, c *CniService, vars map[string]string) (interface{}, error) {
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
	sbID, err := c.lookupSandboxID(cniInfo.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("sandbox lookup failed for containerID %q , error:%v", cniInfo.ContainerID, err)
	}
	epID, err := c.lookupEndpointID(cniInfo.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("endpoint lookup failed for containerID %q, error:%v", cniInfo.ContainerID, err)
	}

	if err = c.endpointLeave(sbID, epID); err != nil {
		return nil, fmt.Errorf("failed to leave endpoint from sandbox for container:%q,sandbox:%q,endpoint:%q, error:%v", cniInfo.ContainerID, sbID, epID, err)
	}

	if err = c.deleteEndpoint(epID); err != nil {
		return nil, fmt.Errorf("failed to delete endpoint %q for container %q,, error:%v",
			epID, cniInfo.ContainerID, err)
	}

	if err = c.deleteSandbox(sbID); err != nil {
		return nil, fmt.Errorf("failed to delete sandbox %q for container %q, error:%v", sbID, cniInfo.ContainerID, err)
	}
	delete(c.endpointIDStore, epID)
	delete(c.sandboxIDStore, sbID)
	return nil, nil
}

func (c *CniService) endpointLeave(sandboxID, endpointID string) error {
	_, _, err := netutils.ReadBody(c.dnetConn.HttpCall("DELETE", "/services/"+endpointID+"/backend/"+sandboxID, nil, nil))
	return err
}

func (c *CniService) deleteSandbox(sandboxID string) error {
	_, _, err := netutils.ReadBody(c.dnetConn.HttpCall("DELETE", "/sandboxes/"+sandboxID, nil, nil))
	return err
}

func (c *CniService) deleteEndpoint(endpointID string) error {
	sd := client.ServiceDelete{Name: endpointID, Force: true}
	_, _, err := netutils.ReadBody(c.dnetConn.HttpCall("DELETE", "/services/"+endpointID, sd, nil))
	return err
}

func (c *CniService) lookupSandboxID(containerID string) (string, error) {
	if id, ok := c.sandboxIDStore[containerID]; ok {
		return id, nil
	}
	obj, _, err := netutils.ReadBody(c.dnetConn.HttpCall("GET", fmt.Sprintf("/sandboxes?partial-container-id=%s", containerID), nil, nil))
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

	c.sandboxIDStore[containerID] = sandboxList[0].ID
	return sandboxList[0].ID, nil
}

func (c *CniService) lookupEndpointID(containerID string) (string, error) {
	if id, ok := c.endpointIDStore[containerID]; ok {
		return id, nil
	}
	return "", fmt.Errorf("endpoint not found")
	//TODO: query libnetwork core if the cache doesnt have it.
}
