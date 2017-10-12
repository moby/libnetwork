package server

import (
	"net"
	"net/http"
	"os"
	"syscall"

	"github.com/docker/libnetwork/netutils"
	"github.com/docker/libnetwork/pkg/cniapi"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const (
	CniServicePort = 9005
)

type CniService struct {
	//TODO k8sClient *APIClient

	listenPath      string
	dnetConn        *netutils.HttpConnection
	sandboxIDStore  map[string]string // containerID to sandboxID mapping
	endpointIDStore map[string]string // containerID to endpointID mapping
}

func NewCniService(sock string, dnetIP string, dnetPort string) (*CniService, error) {
	dnetUrl := dnetIP + ":" + dnetPort
	c := new(CniService)
	c.dnetConn = &netutils.HttpConnection{Addr: dnetUrl, Proto: "tcp"}
	c.listenPath = sock
	c.sandboxIDStore = make(map[string]string)
	c.endpointIDStore = make(map[string]string)
	return c, nil
}

// InitCniService initializes the cni server
func (c *CniService) InitCniService(serverCloseChan chan struct{}) error {
	log.Infof("Starting CNI server")
	// Create http handlers for add and delete pod
	router := mux.NewRouter()
	t := router.Headers("Content-Type", "application/json").Methods("POST").Subrouter()
	t.HandleFunc(cniapi.AddPodUrl, MakeHTTPHandler(c, addPod))
	t.HandleFunc(cniapi.DelPodUrl, MakeHTTPHandler(c, deletePod))
	syscall.Unlink(c.listenPath)
	os.MkdirAll(cniapi.PluginPath, 0700)
	go func() {
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: c.listenPath, Net: "unix"})
		if err != nil {
			panic(err)
		}
		log.Infof("Libnetwork CNI plugin listening on on %s", c.listenPath)
		http.Serve(l, router)
		l.Close()
		close(serverCloseChan)
	}()
	return nil
}

func newCniService() *CniService {
	c := new(CniService)
	c.sandboxIDStore = make(map[string]string)
	c.endpointIDStore = make(map[string]string)
	return c
}
