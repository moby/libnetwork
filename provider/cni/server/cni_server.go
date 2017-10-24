package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/docker/libkv/store"
	"github.com/docker/libkv/store/boltdb"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/docker/libnetwork/client"
	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/netutils"
	"github.com/docker/libnetwork/provider/cni/cniapi"
	cnistore "github.com/docker/libnetwork/provider/cni/store"
	"github.com/docker/libnetwork/types"
)

// CniService hold the cni service information
type CniService struct {
	listenPath string
	dnetConn   *netutils.HTTPConnection
	store      datastore.DataStore
}

var dnetConn *netutils.HTTPConnection

// NewCniService returns a new cni service instance
func NewCniService(sock string, dnetIP string, dnetPort string) (*CniService, error) {
	dnetURL := dnetIP + ":" + dnetPort
	c := new(CniService)
	dnetConn = &netutils.HTTPConnection{Addr: dnetURL, Proto: "tcp"}
	c.listenPath = sock
	return c, nil
}

// Init initializes the cni server
func (c *CniService) Init(serverCloseChan chan struct{}) error {
	log.Infof("Starting CNI server")
	router := mux.NewRouter()
	t := router.Methods("POST").Subrouter()
	t.HandleFunc(cniapi.AddWorkloadURL, MakeHTTPHandler(c, addWorkload))
	t.HandleFunc(cniapi.DelWorkloadURL, MakeHTTPHandler(c, deleteWorkload))

	syscall.Unlink(c.listenPath)
	boltdb.Register()
	store, err := localStore()
	if err != nil {
		return fmt.Errorf("failed to initialize local store: %v", err)
	}
	c.store = store
	go func() {
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: c.listenPath, Net: "unix"})
		if err != nil {
			panic(err)
		}
		log.Infof("Dnet CNI plugin listening on on %s", c.listenPath)
		http.Serve(l, router)
		l.Close()
		close(serverCloseChan)
	}()
	return nil
}

// SetupWorkload sets up the network for the workload
func (c *CniService) SetupWorkload(cniInfo cniapi.CniInfo) (
	ep client.EndpointInfo,
	retErr error,
) {
	// Create a Sandbox
	sbConfig, sbID, err := createSandbox(cniInfo.ContainerID)
	if err != nil {
		return ep, fmt.Errorf("failed to create sandbox for %q: %v", cniInfo.ContainerID, err)
	}
	defer func() {
		if retErr != nil {
			if err := deleteSandbox(sbID); err != nil {
				log.Warnf("failed to delete sandbox %v on setup pod failure , error:%v", sbID, err)
			}
		}
	}()

	// Create an Endpoint
	ep, err = createEndpoint(cniInfo.ContainerID, cniInfo.NetConf)
	if err != nil {
		return ep, fmt.Errorf("failed to create endpoint for %q: %v", cniInfo.ContainerID, err)
	}
	defer func() {
		if retErr != nil {
			if err := deleteEndpoint(ep.ID); err != nil {
				log.Warnf("failed to delete endpoint %v on setup pod failure , error:%v", ep.ID, err)
			}
		}
	}()

	// Attach endpoint to the sandbox
	if err = endpointJoin(sbID, ep.ID, cniInfo.NetNS); err != nil {
		return ep, fmt.Errorf("failed to attach endpoint to sandbox for container:%q,sandbox:%q,endpoint:%q, error:%v", cniInfo.ContainerID, sbID, ep.ID, err)
	}
	defer func() {
		if retErr != nil {
			if err = endpointLeave(sbID, ep.ID); err != nil {
				log.Warnf("failed to detach endpoint %q from sandbox %q , err:%v", ep.ID, sbID, err)
			}
		}
	}()

	if err := c.putMetadataToStore(cniInfo, sbID, ep.ID, sbConfig); err != nil {
		return ep, fmt.Errorf("failed put metadata to store: %v", err)
	}
	return ep, nil
}

// TearDownWorkload tears the networking of the workload
func (c *CniService) TearDownWorkload(cniInfo cniapi.CniInfo) error {
	cniMetadata, err := c.getMetadataFromStore(cniInfo)
	if err != nil {
		log.Errorf("cni workload data not found in plugin store: %v", err)
		// If its not found in store we do not have information regarding this workload.
		// We just return nil. TODO : figure out an alternative if this causes unwanted
		// issues
		return nil
	}
	sbID := cniMetadata.SandboxID
	epID := cniMetadata.EndpointID

	if err = endpointLeave(sbID, epID); err != nil {
		return fmt.Errorf("failed to leave endpoint from sandbox for container:%q,sandbox:%q,endpoint:%q, error:%v", cniInfo.ContainerID, sbID, epID, err)
	}

	if err = deleteEndpoint(epID); err != nil {
		return fmt.Errorf("failed to delete endpoint %q for container %q,, error:%v",
			epID, cniInfo.ContainerID, err)
	}

	if err = deleteSandbox(sbID); err != nil {
		return fmt.Errorf("failed to delete sandbox %q for container %q, error:%v", sbID, cniInfo.ContainerID, err)
	}

	return c.deleteMetadataFromStore(cniMetadata)
}

func (c *CniService) putMetadataToStore(cniInfo cniapi.CniInfo,
	sbID,
	epID string,
	sbConfig client.SandboxCreate,
) error {
	var err error
	cs := &cnistore.CniMetadata{
		ContainerID: cniInfo.ContainerID,
		SandboxID:   sbID,
		EndpointID:  epID,
		SandboxMeta: copySandboxMetadata(sbConfig, cniInfo.NetNS),
	}
	store := c.getstore()
	if store == nil {
		return nil
	}
	if err = store.PutObjectAtomic(cs); err == datastore.ErrKeyModified {
		return types.RetryErrorf("failed to perform atomic write (%v). retry might fix the error", err)
	}

	return err
}

func (c *CniService) deleteMetadataFromStore(cs *cnistore.CniMetadata) error {
	store := c.getstore()
	if store == nil {
		return nil
	}
	return store.DeleteObjectAtomic(cs)
}

func (c *CniService) getMetadataFromStore(cniInfo cniapi.CniInfo) (*cnistore.CniMetadata, error) {
	store := c.getstore()
	if store == nil {
		return nil, nil
	}
	cs := &cnistore.CniMetadata{ContainerID: cniInfo.ContainerID}
	if err := store.GetObject(datastore.Key(cs.Key()...), cs); err != nil {
		if err == datastore.ErrKeyNotFound {
			return nil, fmt.Errorf("failed to find cni metadata from store for %s workload %s",
				cniInfo.ContainerID, err)
		}
		return nil, types.InternalErrorf("could not get pools config from store: %v", err)
	}
	return cs, nil
}

func localStore() (datastore.DataStore, error) {
	return datastore.NewDataStore(datastore.LocalScope, &datastore.ScopeCfg{
		Client: datastore.ScopeClientCfg{
			Provider: string(store.BOLTDB),
			Address:  "/var/run/libnetwork/cnidb.db",
			Config: &store.Config{
				Bucket:            "cni-dnet",
				ConnectionTimeout: 5 * time.Second,
			},
		},
	})
}

// getstore returns store instance
func (c *CniService) getstore() datastore.DataStore {
	return c.store
}

func createSandbox(ContainerID string) (client.SandboxCreate, string, error) {
	sc := client.SandboxCreate{ContainerID: ContainerID, UseExternalKey: true}
	obj, _, err := netutils.ReadBody(dnetConn.HTTPCall("POST", "/sandboxes", sc, nil))
	if err != nil {
		return client.SandboxCreate{}, "", err
	}

	var replyID string
	err = json.Unmarshal(obj, &replyID)
	if err != nil {
		return client.SandboxCreate{}, "", err
	}
	return sc, replyID, nil
}

func createEndpoint(ContainerID string, netConfig cniapi.NetworkConf) (client.EndpointInfo, error) {
	var ep client.EndpointInfo
	// Create network if it doesnt exist. Need to handle refcount to delete
	// network on last workload delete.
	if !networkExists(netConfig.Name) {
		if err := createNetwork(netConfig); err != nil && !strings.Contains(err.Error(), "already exists") {
			return ep, err
		}
	}

	sc := client.ServiceCreate{Name: ContainerID, Network: netConfig.Name, DisableResolution: true}
	obj, _, err := netutils.ReadBody(dnetConn.HTTPCall("POST", "/services", sc, nil))
	if err != nil {
		return ep, err
	}
	err = json.Unmarshal(obj, &ep)
	return ep, err
}

func endpointJoin(sandboxID, endpointID, netns string) (retErr error) {
	nc := client.ServiceAttach{SandboxID: sandboxID, SandboxKey: netns}
	_, _, err := netutils.ReadBody(dnetConn.HTTPCall("POST", "/services/"+endpointID+"/backend", nc, nil))
	return err
}

func networkExists(networkID string) bool {
	obj, statusCode, err := netutils.ReadBody(dnetConn.HTTPCall("GET", "/networks?partial-id="+networkID, nil, nil))
	if err != nil {
		log.Debugf("%s network does not exist:%v \n", networkID, err)
		return false
	}
	if statusCode != http.StatusOK {
		log.Debugf("%s network does not exist \n", networkID)
		return false
	}
	var list []*client.NetworkResource
	err = json.Unmarshal(obj, &list)
	if err != nil {
		return false
	}
	return (len(list) != 0)
}

// createNetwork is a very simple utility to create a default network
// if not present.
//TODO: Need to watch out for parallel createnetwork calls on multiple nodes
func createNetwork(netConf cniapi.NetworkConf) error {
	log.Infof("Creating network %+v \n", netConf)
	driverOpts := make(map[string]string)
	driverOpts["hostaccess"] = ""
	nc := client.NetworkCreate{Name: netConf.Name, ID: netConf.Name, NetworkType: getNetworkType(netConf.Name),
		DriverOpts: driverOpts}
	if ipam := netConf.IPAM; ipam != nil {
		cfg := client.IPAMConf{}
		if ipam.PreferredPool != "" {
			cfg.PreferredPool = ipam.PreferredPool
		}
		if ipam.SubPool != "" {
			cfg.SubPool = ipam.SubPool
		}
		if ipam.Gateway != "" {
			cfg.Gateway = ipam.Gateway
		}
		nc.IPv4Conf = []client.IPAMConf{cfg}
	}
	obj, _, err := netutils.ReadBody(dnetConn.HTTPCall("POST", "/networks", nc, nil))
	if err != nil {
		return err
	}
	var replyID string
	err = json.Unmarshal(obj, &replyID)
	if err != nil {
		return err
	}
	fmt.Printf("Network creation succeeded: %v", replyID)
	return nil
}

func endpointLeave(sandboxID, endpointID string) error {
	log.Infof("Sending EndpointLeave for endpoint %s , sandbox:%s \n", endpointID, sandboxID)
	_, _, err := netutils.ReadBody(dnetConn.HTTPCall("DELETE", "/services/"+endpointID+"/backend/"+sandboxID, nil, nil))
	return err
}

func deleteSandbox(sandboxID string) error {
	log.Infof("Sending deleteSandbox sandbox:%s \n", sandboxID)
	_, _, err := netutils.ReadBody(dnetConn.HTTPCall("DELETE", "/sandboxes/"+sandboxID, nil, nil))
	return err
}

func deleteEndpoint(endpointID string) error {
	log.Infof("Sending deleteEndpoint for endpoint %s \n", endpointID)
	sd := client.ServiceDelete{Name: endpointID, Force: true}
	_, _, err := netutils.ReadBody(dnetConn.HTTPCall("DELETE", "/services/"+endpointID, sd, nil))
	return err
}
