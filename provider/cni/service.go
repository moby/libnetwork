package cni

import (
	"github.com/docker/libnetwork/client"
	"github.com/docker/libnetwork/provider/cni/cniapi"
)

// Service interface for Cni service implementations
type Service interface {
	// Init initializes the cni server
	Init(serverCloseChan chan struct{}) error
	// SetupWorkload setups the CNI workload networking
	SetupWorkload(cniapi.CniInfo) (client.EndpointInfo, error)
	// TearDownWorkload tears down the CNI workload networking
	TearDownWorkload(cniapi.CniInfo) error
}
