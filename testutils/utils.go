package testutils

import (
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/docker/libkv/store"
	"github.com/docker/libkv/store/boltdb"
	"github.com/docker/libnetwork/datastore"
)

const (
	defaultPrefix = "/tmp/libnetwork/test/"
)

var runningInContainer = flag.Bool("incontainer", false, "Indicates if the test is running in a container")

// IsRunningInContainer returns whether the test is running inside a container.
func IsRunningInContainer() bool {
	return (*runningInContainer)
}

func init() {
	boltdb.Register()
}

// RandomLocalStore returns a random bltdb datastore
func RandomLocalStore(component string) (datastore.DataStore, error) {
	tmp, err := ioutil.TempFile("", "libnetwork-")
	if err != nil {
		return nil, fmt.Errorf("Error creating temp file: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("Error closing temp file: %v", err)
	}
	return datastore.NewDataStore(datastore.LocalScope, &datastore.ScopeCfg{
		Client: datastore.ScopeClientCfg{
			Provider: "boltdb",
			Address:  defaultPrefix + component + tmp.Name(),
			Config: &store.Config{
				Bucket:            "libnetwork",
				ConnectionTimeout: 3 * time.Second,
			},
		},
	})
}
