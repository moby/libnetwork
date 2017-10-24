package store

import (
	"encoding/json"

	"github.com/docker/libnetwork/api"
	"github.com/docker/libnetwork/datastore"
	"github.com/sirupsen/logrus"
)

const (
	cniPrefix = "cni"
)

// CniMetadata holds the cni metadata information for a workload
type CniMetadata struct {
	ContainerID string
	SandboxID   string
	EndpointID  string
	SandboxMeta api.SandboxMetadata
	dbIndex     uint64
	dbExists    bool
}

// Key provides the Key to be used in KV Store
func (cs *CniMetadata) Key() []string {
	return []string{cniPrefix, cs.ContainerID}
}

// KeyPrefix returns the immediate parent key that can be used for tree walk
func (cs *CniMetadata) KeyPrefix() []string {
	return []string{cniPrefix}
}

// Value marshals the data to be stored in the KV store
func (cs *CniMetadata) Value() []byte {
	b, err := json.Marshal(cs)
	if err != nil {
		logrus.Warnf("failed to marshal cni store: %v", err)
		return nil
	}
	return b
}

// SetValue unmarshalls the data from the KV store.
func (cs *CniMetadata) SetValue(value []byte) error {
	return json.Unmarshal(value, cs)
}

// Index returns the latest DB Index as seen by this object
func (cs *CniMetadata) Index() uint64 {
	return cs.dbIndex
}

// SetIndex method allows the datastore to store the latest DB Index into this object
func (cs *CniMetadata) SetIndex(index uint64) {
	cs.dbIndex = index
	cs.dbExists = true
}

// Exists method is true if this object has been stored in the DB.
func (cs *CniMetadata) Exists() bool {
	return cs.dbExists
}

// Skip provides a way for a KV Object to avoid persisting it in the KV Store
func (cs *CniMetadata) Skip() bool {
	return false
}

// New returns a new cnimetada KVObjects
func (cs *CniMetadata) New() datastore.KVObject {
	return &CniMetadata{}
}

// CopyTo copy from source to destination KBObject
func (cs *CniMetadata) CopyTo(o datastore.KVObject) error {
	dstCs := o.(*CniMetadata)
	dstCs.ContainerID = cs.ContainerID
	dstCs.SandboxID = cs.SandboxID
	dstCs.EndpointID = cs.EndpointID
	dstCs.SandboxMeta = cs.SandboxMeta
	dstCs.dbIndex = cs.dbIndex
	dstCs.dbExists = cs.dbExists
	return nil
}

// DataScope method returns the storage scope of the datastore
func (cs *CniMetadata) DataScope() string {
	return datastore.LocalScope
}
