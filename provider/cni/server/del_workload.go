package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/docker/libnetwork/provider/cni"
	"github.com/docker/libnetwork/provider/cni/cniapi"
)

func deleteWorkload(w http.ResponseWriter, r *http.Request, c cni.Service, vars map[string]string) (interface{}, error) {
	//TODO: need to explore force cleanup and test for parallel delete workloads
	cniInfo := cniapi.CniInfo{}

	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request: %v", err)
	}

	if err = json.Unmarshal(content, &cniInfo); err != nil {
		return nil, err
	}
	logrus.Infof("Received delete workload request %+v", cniInfo)
	if err := c.TearDownWorkload(cniInfo); err != nil {
		return nil, err
	}
	return nil, nil
}
