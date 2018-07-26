package overlay

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/serf/serf"
	"github.com/sirupsen/logrus"
)

// ovNotify indicates that an endpoint is joining or leaving a network.
// The only valid values for action are "join" and "leave".
type ovNotify struct {
	action string
	ep     *endpoint
	nw     *network
}

type logWriter struct{}

func (l *logWriter) Write(p []byte) (int, error) {
	str := string(p)

	switch {
	case strings.Contains(str, "[WARN]"):
		logrus.Warn(str)
	case strings.Contains(str, "[DEBUG]"):
		logrus.Debug(str)
	case strings.Contains(str, "[INFO]"):
		logrus.Info(str)
	case strings.Contains(str, "[ERR]"):
		logrus.Error(str)
	}

	return len(p), nil
}

// serfInit configures and starts a Serf instance, creates channels
// for communication between Serf and the overlay driver and starts
// a goroutine to multiplex and dispatch messages between them.
func (d *driver) serfInit() error {
	var err error

	config := serf.DefaultConfig()
	config.Init()
	config.MemberlistConfig.BindAddr = d.advertiseAddress

	d.eventCh = make(chan serf.Event, 4)
	config.EventCh = d.eventCh
	config.UserCoalescePeriod = 1 * time.Second
	config.UserQuiescentPeriod = 50 * time.Millisecond

	config.LogOutput = &logWriter{}
	config.MemberlistConfig.LogOutput = config.LogOutput

	s, err := serf.Create(config)
	if err != nil {
		return fmt.Errorf("failed to create cluster node: %v", err)
	}
	defer func() {
		if err != nil {
			s.Shutdown()
		}
	}()

	d.serfInstance = s

	d.notifyCh = make(chan ovNotify)
	d.exitCh = make(chan chan struct{})

	go d.startSerfLoop(d.eventCh, d.notifyCh, d.exitCh)
	return nil
}

// serfJoin attempts to join the Serf cluster at neighIP.
func (d *driver) serfJoin(neighIP string) error {
	if neighIP == "" {
		return fmt.Errorf("no neighbor to join")
	}
	if _, err := d.serfInstance.Join([]string{neighIP}, true); err != nil {
		return fmt.Errorf("Failed to join the cluster at neigh IP %s: %v",
			neighIP, err)
	}
	return nil
}

// notifyEvent pushes an ovNotify event, containing an endpoint join or leave
// message, from the driver to the Serf cluster
func (d *driver) notifyEvent(event ovNotify) {
	ep := event.ep

	ePayload := fmt.Sprintf("%s %s %s %s", event.action, ep.addr.IP.String(),
		net.IP(ep.addr.Mask).String(), ep.mac.String())
	eName := fmt.Sprintf("jl %s %s %s", d.serfInstance.LocalMember().Addr.String(),
		event.nw.id, ep.id)

	if err := d.serfInstance.UserEvent(eName, []byte(ePayload), true); err != nil {
		logrus.Errorf("Sending user event failed: %v\n", err)
	}
}

// processEvent handles a UserEvent containing an endpoint join or
// leave message received from the Serf cluster and dispatches it to the
// appropriate driver function.
func (d *driver) processEvent(u serf.UserEvent) {
	logrus.Debugf("Received user event name:%s, payload:%s LTime:%d \n", u.Name,
		string(u.Payload), uint64(u.LTime))

	var dummy, action, vtepStr, nid, eid, ipStr, maskStr, macStr string
	if _, err := fmt.Sscan(u.Name, &dummy, &vtepStr, &nid, &eid); err != nil {
		fmt.Printf("Failed to scan name string: %v\n", err)
	}

	if _, err := fmt.Sscan(string(u.Payload), &action,
		&ipStr, &maskStr, &macStr); err != nil {
		fmt.Printf("Failed to scan value string: %v\n", err)
	}

	logrus.Debugf("Parsed data = %s/%s/%s/%s/%s/%s\n", nid, eid, vtepStr, ipStr, maskStr, macStr)

	mac, err := net.ParseMAC(macStr)
	if err != nil {
		logrus.Errorf("Failed to parse mac: %v\n", err)
	}

	if d.serfInstance.LocalMember().Addr.String() == vtepStr {
		return
	}

	switch action {
	case "join":
		d.peerAdd(nid, eid, net.ParseIP(ipStr), net.IPMask(net.ParseIP(maskStr).To4()), mac, net.ParseIP(vtepStr), false, false, false)
	case "leave":
		d.peerDelete(nid, eid, net.ParseIP(ipStr), net.IPMask(net.ParseIP(maskStr).To4()), mac, net.ParseIP(vtepStr), false)
	}
}

// processQuery handles a Query from another node requesting information
// about a peer; it is the counterpart of resolvePeer. processQuery looks
// up the requested network and IP in the peer database and returns the
// results to the requesting node via Serf.
func (d *driver) processQuery(q *serf.Query) {
	logrus.Debugf("Received query name:%s, payload:%s\n", q.Name,
		string(q.Payload))

	var nid, ipStr string
	if _, err := fmt.Sscan(string(q.Payload), &nid, &ipStr); err != nil {
		fmt.Printf("Failed to scan query payload string: %v\n", err)
	}

	pKey, pEntry, err := d.peerDbSearch(nid, net.ParseIP(ipStr))
	if err != nil {
		return
	}

	logrus.Debugf("Sending peer query resp mac %v, mask %s, vtep %s", pKey.peerMac, net.IP(pEntry.peerIPMask).String(), pEntry.vtep)
	q.Respond([]byte(fmt.Sprintf("%s %s %s", pKey.peerMac.String(), net.IP(pEntry.peerIPMask).String(), pEntry.vtep.String())))
}

// resolvePeer queries the rest of the cluster to resolve peerIP in
// network nid.   resolvePeer returns an error if the response from the
// cluster is corrupt or if the call times out without receiving a response.
func (d *driver) resolvePeer(nid string, peerIP net.IP) (net.HardwareAddr, net.IPMask, net.IP, error) {
	if d.serfInstance == nil {
		return nil, nil, nil, fmt.Errorf("could not resolve peer: serf instance not initialized")
	}

	qPayload := fmt.Sprintf("%s %s", string(nid), peerIP.String())
	resp, err := d.serfInstance.Query("peerlookup", []byte(qPayload), nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolving peer by querying the cluster failed: %v", err)
	}

	respCh := resp.ResponseCh()
	select {
	case r := <-respCh:
		var macStr, maskStr, vtepStr string
		if _, err := fmt.Sscan(string(r.Payload), &macStr, &maskStr, &vtepStr); err != nil {
			return nil, nil, nil, fmt.Errorf("bad response %q for the resolve query: %v", string(r.Payload), err)
		}

		mac, err := net.ParseMAC(macStr)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to parse mac: %v", err)
		}

		logrus.Debugf("Received peer query response, mac %s, vtep %s, mask %s", macStr, vtepStr, maskStr)
		return mac, net.IPMask(net.ParseIP(maskStr).To4()), net.ParseIP(vtepStr), nil

	case <-time.After(time.Second):
		return nil, nil, nil, fmt.Errorf("timed out resolving peer by querying the cluster")
	}
}

// startSerfLoop multiplexes and dispatches messages between the overlay
// driver and Serf, the gossip layer.
//
// notifyCh carries join and leave messages from the driver.   startSerfLoop
// forwards these to the Serf cluster.
//
// exitCh carries channels from the driver.   On receiving one of these,
// startSerfLoop leaves the Serf cluster, shuts down the local Serf instance
// and closes the channel to inform the caller that shutdown is complete.
//
// eventCh carries join and leave messages (wrapped in UserEvent structs) and
// peerlookup messages (wrapped in Query structs) from Serf.   startSerfLoop
// forwards join and leave messages to the appropriate driver functions; it
// calls peerDB to resolve peerlookup queries and sends the results back
// directly to the requester.
func (d *driver) startSerfLoop(eventCh chan serf.Event, notifyCh chan ovNotify,
	exitCh chan chan struct{}) {

	for {
		select {
		case notify, ok := <-notifyCh:
			if !ok {
				break
			}

			d.notifyEvent(notify)
		case ch, ok := <-exitCh:
			if !ok {
				break
			}

			if err := d.serfInstance.Leave(); err != nil {
				logrus.Errorf("failed leaving the cluster: %v\n", err)
			}

			d.serfInstance.Shutdown()
			close(ch)
			return
		case e, ok := <-eventCh:
			if !ok {
				break
			}

			if e.EventType() == serf.EventQuery {
				d.processQuery(e.(*serf.Query))
				break
			}

			u, ok := e.(serf.UserEvent)
			if !ok {
				break
			}
			d.processEvent(u)
		}
	}
}

// isSerfAlive returns true if serf is initialized and running
func (d *driver) isSerfAlive() bool {
	d.Lock()
	serfInstance := d.serfInstance
	d.Unlock()
	if serfInstance == nil || serfInstance.State() != serf.SerfAlive {
		return false
	}
	return true
}
