package libnetwork

//go:generate protoc -I.:Godeps/_workspace/src/github.com/gogo/protobuf  --gogo_out=import_path=github.com/docker/libnetwork,Mgogoproto/gogo.proto=github.com/gogo/protobuf/gogoproto:. agent.proto

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/stringid"
	"github.com/docker/go-events"
	"github.com/docker/libnetwork/datastore"
	"github.com/docker/libnetwork/discoverapi"
	"github.com/docker/libnetwork/driverapi"
	"github.com/docker/libnetwork/networkdb"
	"github.com/docker/libnetwork/types"
	"github.com/gogo/protobuf/proto"
)

const (
	subsysGossip = "networking:gossip"
	subsysIPSec  = "networking:ipsec"
	keyringSize  = 3
)

// ByTime implements sort.Interface for []*types.EncryptionKey based on
// the LamportTime field.
type ByTime []*types.EncryptionKey

func (b ByTime) Len() int           { return len(b) }
func (b ByTime) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByTime) Less(i, j int) bool { return b[i].LamportTime < b[j].LamportTime }

type agent struct {
	networkDB         *networkdb.NetworkDB
	bindAddr          string
	advertiseAddr     string
	coreCancelFuncs   []func()
	driverCancelFuncs map[string][]func()
	sync.Mutex
}

func getBindAddr(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", fmt.Errorf("failed to find interface %s: %v", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %v", err)
	}

	for _, a := range addrs {
		addr, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		addrIP := addr.IP

		if addrIP.IsLinkLocalUnicast() {
			continue
		}

		return addrIP.String(), nil
	}

	return "", fmt.Errorf("failed to get bind address")
}

func resolveAddr(addrOrInterface string) (string, error) {
	// Try and see if this is a valid IP address
	if net.ParseIP(addrOrInterface) != nil {
		return addrOrInterface, nil
	}

	addr, err := net.ResolveIPAddr("ip", addrOrInterface)
	if err != nil {
		// If not a valid IP address, it should be a valid interface
		return getBindAddr(addrOrInterface)
	}
	return addr.String(), nil
}

func (c *controller) handleKeyChange(keys []*types.EncryptionKey) error {
	drvEnc := discoverapi.DriverEncryptionUpdate{}

	a := c.getAgent()
	if a == nil {
		logrus.Debug("Skipping key change as agent is nil")
		return nil
	}

	// Find the deleted key. If the deleted key was the primary key,
	// a new primary key should be set before removing if from keyring.
	c.Lock()
	added := []byte{}
	deleted := []byte{}
	j := len(c.keys)
	for i := 0; i < j; {
		same := false
		for _, key := range keys {
			if same = key.LamportTime == c.keys[i].LamportTime; same {
				break
			}
		}
		if !same {
			cKey := c.keys[i]
			if cKey.Subsystem == subsysGossip {
				deleted = cKey.Key
			}

			if cKey.Subsystem == subsysIPSec {
				drvEnc.Prune = cKey.Key
				drvEnc.PruneTag = cKey.LamportTime
			}
			c.keys[i], c.keys[j-1] = c.keys[j-1], c.keys[i]
			c.keys[j-1] = nil
			j--
		}
		i++
	}
	c.keys = c.keys[:j]

	// Find the new key and add it to the key ring
	for _, key := range keys {
		same := false
		for _, cKey := range c.keys {
			if same = cKey.LamportTime == key.LamportTime; same {
				break
			}
		}
		if !same {
			c.keys = append(c.keys, key)
			if key.Subsystem == subsysGossip {
				added = key.Key
			}

			if key.Subsystem == subsysIPSec {
				drvEnc.Key = key.Key
				drvEnc.Tag = key.LamportTime
			}
		}
	}
	c.Unlock()

	if len(added) > 0 {
		a.networkDB.SetKey(added)
	}

	key, tag, err := c.getPrimaryKeyTag(subsysGossip)
	if err != nil {
		return err
	}
	a.networkDB.SetPrimaryKey(key)

	key, tag, err = c.getPrimaryKeyTag(subsysIPSec)
	if err != nil {
		return err
	}
	drvEnc.Primary = key
	drvEnc.PrimaryTag = tag

	if len(deleted) > 0 {
		a.networkDB.RemoveKey(deleted)
	}

	c.drvRegistry.WalkDrivers(func(name string, driver driverapi.Driver, capability driverapi.Capability) bool {
		err := driver.DiscoverNew(discoverapi.EncryptionKeysUpdate, drvEnc)
		if err != nil {
			logrus.Warnf("Failed to update datapath keys in driver %s: %v", name, err)
		}
		return false
	})

	return nil
}

func (c *controller) agentSetup() error {
	c.Lock()
	clusterProvider := c.cfg.Daemon.ClusterProvider
	agent := c.agent
	c.Unlock()
	bindAddr := clusterProvider.GetLocalAddress()
	advAddr := clusterProvider.GetAdvertiseAddress()
	remote := clusterProvider.GetRemoteAddress()
	remoteAddr, _, _ := net.SplitHostPort(remote)
	listen := clusterProvider.GetListenAddress()
	listenAddr, _, _ := net.SplitHostPort(listen)

	logrus.Infof("Initializing Libnetwork Agent Listen-Addr=%s Local-addr=%s Adv-addr=%s Remote-addr =%s", listenAddr, bindAddr, advAddr, remoteAddr)
	if advAddr != "" && agent == nil {
		if err := c.agentInit(listenAddr, bindAddr, advAddr); err != nil {
			logrus.Errorf("Error in agentInit : %v", err)
		} else {
			c.drvRegistry.WalkDrivers(func(name string, driver driverapi.Driver, capability driverapi.Capability) bool {
				if capability.DataScope == datastore.GlobalScope {
					c.agentDriverNotify(driver)
				}
				return false
			})
		}
	}

	if remoteAddr != "" {
		if err := c.agentJoin(remoteAddr); err != nil {
			logrus.Errorf("Error in joining gossip cluster : %v(join will be retried in background)", err)
		}
	}

	c.Lock()
	if c.agent != nil && c.agentInitDone != nil {
		close(c.agentInitDone)
		c.agentInitDone = nil
	}
	c.Unlock()

	return nil
}

// For a given subsystem getKeys sorts the keys by lamport time and returns
// slice of keys and lamport time which can used as a unique tag for the keys
func (c *controller) getKeys(subsys string) ([][]byte, []uint64) {
	c.Lock()
	defer c.Unlock()

	sort.Sort(ByTime(c.keys))

	keys := [][]byte{}
	tags := []uint64{}
	for _, key := range c.keys {
		if key.Subsystem == subsys {
			keys = append(keys, key.Key)
			tags = append(tags, key.LamportTime)
		}
	}

	keys[0], keys[1] = keys[1], keys[0]
	tags[0], tags[1] = tags[1], tags[0]
	return keys, tags
}

// getPrimaryKeyTag returns the primary key for a given subsystem from the
// list of sorted key and the associated tag
func (c *controller) getPrimaryKeyTag(subsys string) ([]byte, uint64, error) {
	c.Lock()
	defer c.Unlock()
	sort.Sort(ByTime(c.keys))
	keys := []*types.EncryptionKey{}
	for _, key := range c.keys {
		if key.Subsystem == subsys {
			keys = append(keys, key)
		}
	}
	return keys[1].Key, keys[1].LamportTime, nil
}

func (c *controller) agentInit(listenAddr, bindAddrOrInterface, advertiseAddr string) error {
	if !c.isAgent() {
		return nil
	}

	bindAddr, err := resolveAddr(bindAddrOrInterface)
	if err != nil {
		return err
	}

	keys, tags := c.getKeys(subsysGossip)
	hostname, _ := os.Hostname()
	nodeName := hostname + "-" + stringid.TruncateID(stringid.GenerateRandomID())
	logrus.Info("Gossip cluster hostname ", nodeName)

	nDB, err := networkdb.New(&networkdb.Config{
		BindAddr:      listenAddr,
		AdvertiseAddr: advertiseAddr,
		NodeName:      nodeName,
		Keys:          keys,
	})

	if err != nil {
		return err
	}

	var cancelList []func()
	ch, cancel := nDB.Watch("endpoint_table", "", "")
	cancelList = append(cancelList, cancel)
	nodeCh, cancel := nDB.Watch(networkdb.NodeTable, "", "")
	cancelList = append(cancelList, cancel)

	c.Lock()
	c.agent = &agent{
		networkDB:         nDB,
		bindAddr:          bindAddr,
		advertiseAddr:     advertiseAddr,
		coreCancelFuncs:   cancelList,
		driverCancelFuncs: make(map[string][]func()),
	}
	c.Unlock()

	go c.handleTableEvents(ch, c.handleEpTableEvent)
	go c.handleTableEvents(nodeCh, c.handleNodeTableEvent)

	drvEnc := discoverapi.DriverEncryptionConfig{}
	keys, tags = c.getKeys(subsysIPSec)
	drvEnc.Keys = keys
	drvEnc.Tags = tags

	c.drvRegistry.WalkDrivers(func(name string, driver driverapi.Driver, capability driverapi.Capability) bool {
		err := driver.DiscoverNew(discoverapi.EncryptionKeysConfig, drvEnc)
		if err != nil {
			logrus.Warnf("Failed to set datapath keys in driver %s: %v", name, err)
		}
		return false
	})

	c.WalkNetworks(joinCluster)

	return nil
}

func (c *controller) agentJoin(remote string) error {
	agent := c.getAgent()
	if agent == nil {
		return nil
	}
	return agent.networkDB.Join([]string{remote})
}

func (c *controller) agentDriverNotify(d driverapi.Driver) {
	agent := c.getAgent()
	if agent == nil {
		return
	}

	d.DiscoverNew(discoverapi.NodeDiscovery, discoverapi.NodeDiscoveryData{
		Address:     agent.advertiseAddr,
		BindAddress: agent.bindAddr,
		Self:        true,
	})

	drvEnc := discoverapi.DriverEncryptionConfig{}
	keys, tags := c.getKeys(subsysIPSec)
	drvEnc.Keys = keys
	drvEnc.Tags = tags

	c.drvRegistry.WalkDrivers(func(name string, driver driverapi.Driver, capability driverapi.Capability) bool {
		err := driver.DiscoverNew(discoverapi.EncryptionKeysConfig, drvEnc)
		if err != nil {
			logrus.Warnf("Failed to set datapath keys in driver %s: %v", name, err)
		}
		return false
	})

}

func (c *controller) agentClose() {
	// Acquire current agent instance and reset its pointer
	// then run closing functions
	c.Lock()
	agent := c.agent
	c.agent = nil
	c.Unlock()

	if agent == nil {
		return
	}

	var cancelList []func()

	agent.Lock()
	for _, cancelFuncs := range agent.driverCancelFuncs {
		for _, cancel := range cancelFuncs {
			cancelList = append(cancelList, cancel)
		}
	}

	// Add also the cancel functions for the network db
	for _, cancel := range agent.coreCancelFuncs {
		cancelList = append(cancelList, cancel)
	}
	agent.Unlock()

	for _, cancel := range cancelList {
		cancel()
	}

	agent.networkDB.Close()
}

func (n *network) isClusterEligible() bool {
	if n.driverScope() != datastore.GlobalScope {
		return false
	}
	return n.getController().getAgent() != nil
}

func (n *network) joinCluster() error {
	if !n.isClusterEligible() {
		return nil
	}

	agent := n.getController().getAgent()
	if agent == nil {
		return nil
	}

	return agent.networkDB.JoinNetwork(n.ID())
}

func (n *network) leaveCluster() error {
	if !n.isClusterEligible() {
		return nil
	}

	agent := n.getController().getAgent()
	if agent == nil {
		return nil
	}

	return agent.networkDB.LeaveNetwork(n.ID())
}

func (ep *endpoint) addDriverInfoToCluster() error {
	n := ep.getNetwork()
	if !n.isClusterEligible() {
		return nil
	}
	if ep.joinInfo == nil {
		return nil
	}

	agent := n.getController().getAgent()
	if agent == nil {
		return nil
	}

	for _, te := range ep.joinInfo.driverTableEntries {
		if err := agent.networkDB.CreateEntry(te.tableName, n.ID(), te.key, te.value); err != nil {
			return err
		}
	}
	return nil
}

func (ep *endpoint) deleteDriverInfoFromCluster() error {
	n := ep.getNetwork()
	if !n.isClusterEligible() {
		return nil
	}
	if ep.joinInfo == nil {
		return nil
	}

	agent := n.getController().getAgent()
	if agent == nil {
		return nil
	}

	for _, te := range ep.joinInfo.driverTableEntries {
		if err := agent.networkDB.DeleteEntry(te.tableName, n.ID(), te.key); err != nil {
			return err
		}
	}
	return nil
}

func (ep *endpoint) addServiceInfoToCluster(sb *sandbox) error {
	n := ep.getNetwork()
	if !n.isClusterEligible() {
		return nil
	}

	sb.Service.Lock()
	defer sb.Service.Unlock()
	logrus.Debugf("addServiceInfoToCluster START for %s %s", ep.svcName, ep.ID())

	// Check that the endpoint is still present on the sandbox before adding it to the service discovery.
	// This is to handle a race between the EnableService and the sbLeave
	// It is possible that the EnableService starts, fetches the list of the endpoints and
	// by the time the addServiceInfoToCluster is called the endpoint got removed from the sandbox
	// The risk is that the deleteServiceInfoToCluster happens before the addServiceInfoToCluster.
	// This check under the Service lock of the sandbox ensure the correct behavior.
	// If the addServiceInfoToCluster arrives first may find or not the endpoint and will proceed or exit
	// but in any case the deleteServiceInfoToCluster will follow doing the cleanup if needed.
	// In case the deleteServiceInfoToCluster arrives first, this one is happening after the endpoint is
	// removed from the list, in this situation the delete will bail out not finding any data to cleanup
	// and the add will bail out not finding the endpoint on the sandbox.
	if e := sb.getEndpoint(ep.ID()); e == nil {
		logrus.Warnf("addServiceInfoToCluster suppressing service resolution ep is not anymore in the sandbox %s", ep.ID())
		return nil
	}

	c := n.getController()
	agent := c.getAgent()

	if !ep.isAnonymous() && ep.Iface().Address() != nil {
		var ingressPorts []*PortConfig
		if ep.svcID != "" {
			// This is a task part of a service
			// Gossip ingress ports only in ingress network.
			if n.ingress {
				ingressPorts = ep.ingressPorts
			}
			if err := c.addServiceBinding(ep.svcName, ep.svcID, n.ID(), ep.ID(), ep.Name(), ep.virtualIP, ingressPorts, ep.svcAliases, ep.myAliases, ep.Iface().Address().IP, "addServiceInfoToCluster"); err != nil {
				return err
			}
		} else {
			// This is a container simply attached to an attachable network
			if err := c.addContainerNameResolution(n.ID(), ep.ID(), ep.Name(), ep.myAliases, ep.Iface().Address().IP, "addServiceInfoToCluster"); err != nil {
				return err
			}
		}

		buf, err := proto.Marshal(&EndpointRecord{
			Name:         ep.Name(),
			ServiceName:  ep.svcName,
			ServiceID:    ep.svcID,
			VirtualIP:    ep.virtualIP.String(),
			IngressPorts: ingressPorts,
			Aliases:      ep.svcAliases,
			TaskAliases:  ep.myAliases,
			EndpointIP:   ep.Iface().Address().IP.String(),
		})
		if err != nil {
			return err
		}

		if agent != nil {
			if err := agent.networkDB.CreateEntry("endpoint_table", n.ID(), ep.ID(), buf); err != nil {
				logrus.Warnf("addServiceInfoToCluster NetworkDB CreateEntry failed for %s %s err:%s", ep.id, n.id, err)
				return err
			}
		}
	}

	logrus.Debugf("addServiceInfoToCluster END for %s %s", ep.svcName, ep.ID())

	return nil
}

func (ep *endpoint) deleteServiceInfoFromCluster(sb *sandbox, method string) error {
	n := ep.getNetwork()
	if !n.isClusterEligible() {
		return nil
	}

	sb.Service.Lock()
	defer sb.Service.Unlock()
	logrus.Debugf("deleteServiceInfoFromCluster from %s START for %s %s", method, ep.svcName, ep.ID())

	c := n.getController()
	agent := c.getAgent()

	if !ep.isAnonymous() && ep.Iface().Address() != nil {
		// Delete the entry from network DB, this will trigger a notification to other nodes
		if agent != nil {
			if err := agent.networkDB.DeleteEntry("endpoint_table", n.ID(), ep.ID()); err != nil {
				logrus.Warnf("deleteServiceInfoFromCluster NetworkDB DeleteEntry failed for %s %s err:%s", ep.id, n.id, err)
			}
		}
		// Update locally
		if ep.svcID != "" {
			// This is a task part of a service
			var ingressPorts []*PortConfig
			if n.ingress {
				ingressPorts = ep.ingressPorts
			}
			if err := c.rmServiceBinding(ep.svcName, ep.svcID, n.ID(), ep.ID(), ep.Name(), ep.virtualIP, ingressPorts, ep.svcAliases, ep.myAliases, ep.Iface().Address().IP, "deleteServiceInfoFromCluster", true); err != nil {
				return err
			}
		} else {
			// This is a container simply attached to an attachable network
			if err := c.delContainerNameResolution(n.ID(), ep.ID(), ep.Name(), ep.myAliases, ep.Iface().Address().IP, "deleteServiceInfoFromCluster"); err != nil {
				return err
			}
		}
	}

	logrus.Debugf("deleteServiceInfoFromCluster from %s END for %s %s", method, ep.svcName, ep.ID())

	return nil
}

func (n *network) addDriverWatches() {
	if !n.isClusterEligible() {
		return
	}

	c := n.getController()
	agent := c.getAgent()
	if agent == nil {
		return
	}
	for _, tableName := range n.driverTables {
		ch, cancel := agent.networkDB.Watch(tableName, n.ID(), "")
		agent.Lock()
		agent.driverCancelFuncs[n.ID()] = append(agent.driverCancelFuncs[n.ID()], cancel)
		agent.Unlock()
		go c.handleTableEvents(ch, n.handleDriverTableEvent)
		d, err := n.driver(false)
		if err != nil {
			logrus.Errorf("Could not resolve driver %s while walking driver tabl: %v", n.networkType, err)
			return
		}

		agent.networkDB.WalkTable(tableName, func(nid, key string, value []byte) bool {
			if nid == n.ID() {
				d.EventNotify(driverapi.Create, nid, tableName, key, value)
			}

			return false
		})
	}
}

func (n *network) cancelDriverWatches() {
	if !n.isClusterEligible() {
		return
	}

	agent := n.getController().getAgent()
	if agent == nil {
		return
	}

	agent.Lock()
	cancelFuncs := agent.driverCancelFuncs[n.ID()]
	delete(agent.driverCancelFuncs, n.ID())
	agent.Unlock()

	for _, cancel := range cancelFuncs {
		cancel()
	}
}

func (c *controller) handleTableEvents(ch *events.Channel, fn func(events.Event)) {
	for {
		select {
		case ev := <-ch.C:
			fn(ev)
		case <-ch.Done():
			return
		}
	}
}

func (n *network) handleDriverTableEvent(ev events.Event) {
	d, err := n.driver(false)
	if err != nil {
		logrus.Errorf("Could not resolve driver %s while handling driver table event: %v", n.networkType, err)
		return
	}

	var (
		etype driverapi.EventType
		tname string
		key   string
		value []byte
	)

	switch event := ev.(type) {
	case networkdb.CreateEvent:
		tname = event.Table
		key = event.Key
		value = event.Value
		etype = driverapi.Create
	case networkdb.DeleteEvent:
		tname = event.Table
		key = event.Key
		value = event.Value
		etype = driverapi.Delete
	case networkdb.UpdateEvent:
		tname = event.Table
		key = event.Key
		value = event.Value
		etype = driverapi.Delete
	}

	d.EventNotify(etype, n.ID(), tname, key, value)
}

func (c *controller) handleNodeTableEvent(ev events.Event) {
	var (
		value    []byte
		isAdd    bool
		nodeAddr networkdb.NodeAddr
	)
	switch event := ev.(type) {
	case networkdb.CreateEvent:
		value = event.Value
		isAdd = true
	case networkdb.DeleteEvent:
		value = event.Value
	case networkdb.UpdateEvent:
		logrus.Errorf("Unexpected update node table event = %#v", event)
	}

	err := json.Unmarshal(value, &nodeAddr)
	if err != nil {
		logrus.Errorf("Error unmarshalling node table event %v", err)
		return
	}
	c.processNodeDiscovery([]net.IP{nodeAddr.Addr}, isAdd)

}

func (c *controller) handleEpTableEvent(ev events.Event) {
	var (
		nid   string
		eid   string
		value []byte
		isAdd bool
		epRec EndpointRecord
	)

	switch event := ev.(type) {
	case networkdb.CreateEvent:
		nid = event.NetworkID
		eid = event.Key
		value = event.Value
		isAdd = true
	case networkdb.DeleteEvent:
		nid = event.NetworkID
		eid = event.Key
		value = event.Value
	case networkdb.UpdateEvent:
		logrus.Errorf("Unexpected update service table event = %#v", event)
		return
	}

	err := proto.Unmarshal(value, &epRec)
	if err != nil {
		logrus.Errorf("Failed to unmarshal service table value: %v", err)
		return
	}

	containerName := epRec.Name
	svcName := epRec.ServiceName
	svcID := epRec.ServiceID
	vip := net.ParseIP(epRec.VirtualIP)
	ip := net.ParseIP(epRec.EndpointIP)
	ingressPorts := epRec.IngressPorts
	serviceAliases := epRec.Aliases
	taskAliases := epRec.TaskAliases

	if containerName == "" || ip == nil {
		logrus.Errorf("Invalid endpoint name/ip received while handling service table event %s", value)
		return
	}

	if isAdd {
		logrus.Debugf("handleEpTableEvent ADD %s R:%v", eid, epRec)
		if svcID != "" {
			// This is a remote task part of a service
			if err := c.addServiceBinding(svcName, svcID, nid, eid, containerName, vip, ingressPorts, serviceAliases, taskAliases, ip, "handleEpTableEvent"); err != nil {
				logrus.Errorf("failed adding service binding for %s epRec:%v err:%s", eid, epRec, err)
				return
			}
		} else {
			// This is a remote container simply attached to an attachable network
			if err := c.addContainerNameResolution(nid, eid, containerName, taskAliases, ip, "handleEpTableEvent"); err != nil {
				logrus.Errorf("failed adding service binding for %s epRec:%v err:%s", eid, epRec, err)
			}
		}
	} else {
		logrus.Debugf("handleEpTableEvent DEL %s R:%v", eid, epRec)
		if svcID != "" {
			// This is a remote task part of a service
			if err := c.rmServiceBinding(svcName, svcID, nid, eid, containerName, vip, ingressPorts, serviceAliases, taskAliases, ip, "handleEpTableEvent", true); err != nil {
				logrus.Errorf("failed removing service binding for %s epRec:%v err:%s", eid, epRec, err)
				return
			}
		} else {
			// This is a remote container simply attached to an attachable network
			if err := c.delContainerNameResolution(nid, eid, containerName, taskAliases, ip, "handleEpTableEvent"); err != nil {
				logrus.Errorf("failed adding service binding for %s epRec:%v err:%s", eid, epRec, err)
			}
		}
	}
}
