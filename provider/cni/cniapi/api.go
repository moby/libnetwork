package cniapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
	log "github.com/sirupsen/logrus"

	"github.com/docker/libnetwork"
	"github.com/docker/libnetwork/api"
)

const (
	// AddWorkloadURL url endpoint to add workload
	AddWorkloadURL = "/AddWorkload"
	// DelWorkloadURL url endpoint to delete workload
	DelWorkloadURL = "/DelWorkload"
	// GetActiveWorkloads url endpoint to fetch active workload sandboxes
	GetActiveWorkloads = "/ActiveWorkloads"
	// DnetCNISock is dnet cni sidecar sock file
	DnetCNISock = "/var/run/cniserver.sock"
)

// DnetCniClient  is the cni client connection information
type DnetCniClient struct {
	url        string
	httpClient *http.Client
}

// NetworkConf is the cni network configuration information
type NetworkConf struct {
	CNIVersion   string          `json:"cniVersion,omitempty"`
	Name         string          `json:"name,omitempty"`
	Type         string          `json:"type,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	IPAM         *IPAMConf       `json:"ipam,omitempty"`
	DNS          types.DNS       `json:"dns"`
}

// IPAMConf is the cni network IPAM configuration information
type IPAMConf struct {
	Type          string `json:"type,omitempty"`
	PreferredPool string `json:"preferred-pool,omitempty"`
	SubPool       string `json:"sub-pool,omitempty"`
	Gateway       string `json:"gateway,omitempty"`
}

// CniInfo represents the cni information for a cni transaction
type CniInfo struct {
	ContainerID string
	NetNS       string
	IfName      string
	NetConf     NetworkConf
	Metadata    map[string]string
}

func unixDial(proto, addr string) (conn net.Conn, err error) {
	sock := DnetCNISock
	return net.Dial("unix", sock)
}

// NewDnetCniClient returns a well formed cni client
func NewDnetCniClient() *DnetCniClient {
	c := new(DnetCniClient)
	c.url = "http://localhost"
	c.httpClient = &http.Client{
		Transport: &http.Transport{
			Dial: unixDial,
		},
	}
	return c
}

// AddWorkload setups up the sandbox and endpoint for the infra container in a workload
func (l *DnetCniClient) AddWorkload(args *skel.CmdArgs) (*current.Result, error) {
	var data current.Result
	log.Infof("Sending add workload request %+v", args)
	workloadNetInfo, err := validateWorkloadNetworkInfo(args, true)
	if err != nil {
		return nil, fmt.Errorf("failed to valid cni arguments, error: %v", err)
	}
	buf, err := json.Marshal(workloadNetInfo)
	if err != nil {
		return nil, err
	}
	body := bytes.NewBuffer(buf)
	url := l.url + AddWorkloadURL
	r, err := l.httpClient.Post(url, "application/json", body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	switch {
	case r.StatusCode == int(404):
		return nil, fmt.Errorf("page not found")

	case r.StatusCode == int(403):
		return nil, fmt.Errorf("access denied")

	case r.StatusCode == int(500):
		info, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(info, &data)
		if err != nil {
			return nil, err
		}
		return &data, fmt.Errorf("Internal Server Error")

	case r.StatusCode != int(200):
		log.Errorf("POST Status '%s' status code %d \n", r.Status, r.StatusCode)
		return nil, fmt.Errorf("%s", r.Status)
	}

	response, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(response, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// DeleteWorkload tears the sandbox and endpoint created for the workload.
func (l *DnetCniClient) DeleteWorkload(args *skel.CmdArgs) error {
	log.Infof("Sending delete Workload request %+v", args)
	workloadNetInfo, err := validateWorkloadNetworkInfo(args, false)
	if err != nil {
		return fmt.Errorf("failed to validate cni arguments, error: %v", err)
	}
	buf, err := json.Marshal(workloadNetInfo)
	if err != nil {
		return err
	}
	body := bytes.NewBuffer(buf)
	url := l.url + DelWorkloadURL
	r, err := l.httpClient.Post(url, "application/json", body)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return nil
}

// FetchActiveSandboxes returns a list of active workloads and their sandboxIDs
func (l *DnetCniClient) FetchActiveSandboxes() (map[string]interface{}, error) {
	log.Infof("Requesting for for active sandboxes")
	var sandboxes map[string]api.SandboxMetadata
	url := l.url + GetActiveWorkloads
	r, err := l.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed during http get :%v", err)
	}
	defer r.Body.Close()
	response, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(response, &sandboxes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode http response: %v", err)
	}
	result := make(map[string]interface{})
	for sb, meta := range sandboxes {
		result[sb] = parseConfigOptions(meta)
	}
	return result, nil
}

func parseConfigOptions(meta api.SandboxMetadata) []libnetwork.SandboxOption {
	var sbOptions []libnetwork.SandboxOption
	if meta.UseExternalKey {
		sbOptions = append(sbOptions, libnetwork.OptionUseExternalKey())
	}
	if meta.ExternalKey != "" {
		sbOptions = append(sbOptions, libnetwork.OptionExternalKey(meta.ExternalKey))
	}
	return sbOptions
}

func validateWorkloadNetworkInfo(args *skel.CmdArgs, add bool) (*CniInfo, error) {
	rt := new(CniInfo)
	if args.ContainerID == "" {
		return nil, fmt.Errorf("containerID empty")
	}
	rt.ContainerID = args.ContainerID
	if add {
		if args.Netns == "" {
			return nil, fmt.Errorf("network namespace not present")
		}
		_, err := ns.GetNS(args.Netns)
		if err != nil {
			return nil, err
		}
		rt.NetNS = args.Netns
	}
	if args.IfName == "" {
		rt.IfName = "eth1"
	} else {
		rt.IfName = args.IfName
	}
	var netConf struct {
		NetworkConf
	}
	if err := json.Unmarshal(args.StdinData, &netConf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal network configuration :%v", err)
	}
	rt.NetConf = netConf.NetworkConf
	if args.Args != "" {
		rt.Metadata = getMetadataFromArgs(args.Args)
	}
	return rt, nil
}

func getMetadataFromArgs(args string) map[string]string {
	m := make(map[string]string)
	for _, a := range strings.Split(args, ";") {
		if strings.Contains(a, "=") {
			kvPair := strings.Split(a, "=")
			m[strings.TrimSpace(kvPair[0])] = strings.TrimSpace(kvPair[1])
		} else {
			m[a] = ""
		}
	}
	return m
}
