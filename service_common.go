// +build linux windows

package libnetwork

import (
	"context"
	"net"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libnetwork/common"
)

func (c *controller) addEndpointNameResolution(ctx context.Context, svcName, svcID, nID, eID, containerName string, vip net.IP, serviceAliases, taskAliases []string, ip net.IP, addService bool) error {
	n, err := c.NetworkByID(nID)
	if err != nil {
		return err
	}

	logrus.Debugf("addEndpointNameResolution %s %s add_service:%t sAliases:%v tAliases:%v", eID, svcName, addService, serviceAliases, taskAliases)

	// Add container resolution mappings
	c.addContainerNameResolution(ctx, nID, eID, containerName, taskAliases, ip)

	serviceID := svcID
	if serviceID == "" {
		// This is the case of a normal container not part of a service
		serviceID = eID
	}

	// Add endpoint IP to special "tasks.svc_name" so that the applications have access to DNS RR.
	n.(*network).addSvcRecords(ctx, eID, "tasks."+svcName, serviceID, ip, nil, false)
	for _, alias := range serviceAliases {
		n.(*network).addSvcRecords(ctx, eID, "tasks."+alias, serviceID, ip, nil, false)
	}

	// Add service name to vip in DNS, if vip is valid. Otherwise resort to DNS RR
	if len(vip) == 0 {
		n.(*network).addSvcRecords(ctx, eID, svcName, serviceID, ip, nil, false)
		for _, alias := range serviceAliases {
			n.(*network).addSvcRecords(ctx, eID, alias, serviceID, ip, nil, false)
		}
	}

	if addService && len(vip) != 0 {
		n.(*network).addSvcRecords(ctx, eID, svcName, serviceID, vip, nil, false)
		for _, alias := range serviceAliases {
			n.(*network).addSvcRecords(ctx, eID, alias, serviceID, vip, nil, false)
		}
	}

	return nil
}

func (c *controller) addContainerNameResolution(ctx context.Context, nID, eID, containerName string, taskAliases []string, ip net.IP) error {
	n, err := c.NetworkByID(nID)
	if err != nil {
		return err
	}
	logrus.Debugf("addContainerNameResolution %s %s", eID, containerName)

	// Add resolution for container name
	n.(*network).addSvcRecords(ctx, eID, containerName, eID, ip, nil, true)

	// Add resolution for taskaliases
	for _, alias := range taskAliases {
		n.(*network).addSvcRecords(ctx, eID, alias, eID, ip, nil, true)
	}

	return nil
}

func (c *controller) deleteEndpointNameResolution(ctx context.Context, svcName, svcID, nID, eID, containerName string, vip net.IP, serviceAliases, taskAliases []string, ip net.IP, rmService, multipleEntries bool) error {
	n, err := c.NetworkByID(nID)
	if err != nil {
		return err
	}

	logrus.Debugf("deleteEndpointNameResolution %s %s rm_service:%t suppress:%t sAliases:%v tAliases:%v", eID, svcName, rmService, multipleEntries, serviceAliases, taskAliases)

	// Delete container resolution mappings
	c.delContainerNameResolution(ctx, nID, eID, containerName, taskAliases, ip)

	serviceID := svcID
	if serviceID == "" {
		// This is the case of a normal container not part of a service
		serviceID = eID
	}

	// Delete the special "tasks.svc_name" backend record.
	if !multipleEntries {
		n.(*network).deleteSvcRecords(ctx, eID, "tasks."+svcName, serviceID, ip, nil, false)
		for _, alias := range serviceAliases {
			n.(*network).deleteSvcRecords(ctx, eID, "tasks."+alias, serviceID, ip, nil, false)
		}
	}

	// If we are doing DNS RR delete the endpoint IP from DNS record right away.
	if !multipleEntries && len(vip) == 0 {
		n.(*network).deleteSvcRecords(ctx, eID, svcName, serviceID, ip, nil, false)
		for _, alias := range serviceAliases {
			n.(*network).deleteSvcRecords(ctx, eID, alias, serviceID, ip, nil, false)
		}
	}

	// Remove the DNS record for VIP only if we are removing the service
	if rmService && len(vip) != 0 && !multipleEntries {
		n.(*network).deleteSvcRecords(ctx, eID, svcName, serviceID, vip, nil, false)
		for _, alias := range serviceAliases {
			n.(*network).deleteSvcRecords(ctx, eID, alias, serviceID, vip, nil, false)
		}
	}

	return nil
}

func (c *controller) delContainerNameResolution(ctx context.Context, nID, eID, containerName string, taskAliases []string, ip net.IP) error {
	n, err := c.NetworkByID(nID)
	if err != nil {
		return err
	}
	logrus.Debugf("delContainerNameResolution %s %s", eID, containerName)

	// Delete resolution for container name
	n.(*network).deleteSvcRecords(ctx, eID, containerName, eID, ip, nil, true)

	// Delete resolution for taskaliases
	for _, alias := range taskAliases {
		n.(*network).deleteSvcRecords(ctx, eID, alias, eID, ip, nil, true)
	}

	return nil
}

func newService(name string, id string, ingressPorts []*PortConfig, serviceAliases []string) *service {
	return &service{
		name:          name,
		id:            id,
		ingressPorts:  ingressPorts,
		loadBalancers: make(map[string]*loadBalancer),
		aliases:       serviceAliases,
		ipToEndpoint:  common.NewSetMatrix(),
	}
}

func (c *controller) getLBIndex(sid, nid string, ingressPorts []*PortConfig) int {
	skey := serviceKey{
		id:    sid,
		ports: portConfigs(ingressPorts).String(),
	}
	c.Lock()
	s, ok := c.serviceBindings[skey]
	c.Unlock()

	if !ok {
		return 0
	}

	s.Lock()
	lb := s.loadBalancers[nid]
	s.Unlock()

	return int(lb.fwMark)
}

func (c *controller) cleanupServiceBindings(cleanupNID string) {
	var cleanupFuncs []func()

	logrus.Debugf("cleanupServiceBindings for %s", cleanupNID)
	c.Lock()
	services := make([]*service, 0, len(c.serviceBindings))
	for _, s := range c.serviceBindings {
		services = append(services, s)
	}
	c.Unlock()

	for _, s := range services {
		s.Lock()
		// Skip the serviceBindings that got deleted
		if s.deleted {
			s.Unlock()
			continue
		}
		ctx := context.WithValue(context.Background(), callerCtxKey, "cleanupServiceBindings")
		for nid, lb := range s.loadBalancers {
			if cleanupNID != "" && nid != cleanupNID {
				continue
			}

			// The network is being deleted, erase all the associated service discovery records
			// TODO(fcrisciani) separate the Load Balancer from the Service discovery, this operation
			// can be done safely here, but the rmServiceBinding is still keeping consistency in the
			// data structures that are tracking the endpoint to IP mapping.
			c.Lock()
			logrus.Debugf("cleanupServiceBindings erasing the svcRecords for %s", nid)
			delete(c.svcRecords, nid)
			c.Unlock()

			for eid, ip := range lb.backEnds {
				epID := eid
				epIP := ip
				service := s
				loadBalancer := lb
				networkID := nid
				cleanupFuncs = append(cleanupFuncs, func() {
					// ContainerName and taskAliases are not available here, this is still fine because the Service discovery
					// cleanup already happened before. The only thing that rmServiceBinding is still doing here a part from the Load
					// Balancer bookeeping, is to keep consistent the mapping of endpoint to IP.
					if err := c.rmServiceBinding(ctx, service.name, service.id, networkID, epID, "", loadBalancer.vip,
						service.ingressPorts, service.aliases, []string{}, epIP, false); err != nil {
						logrus.Errorf("Failed to remove service bindings for service %s network %s endpoint %s while cleanup: %v",
							service.id, networkID, epID, err)
					}
				})
			}
		}
		s.Unlock()
	}

	for _, f := range cleanupFuncs {
		f()
	}

}

func (c *controller) addServiceBinding(ctx context.Context, svcName, svcID, nID, eID, containerName string, vip net.IP, ingressPorts []*PortConfig, serviceAliases, taskAliases []string, ip net.IP) error {
	var addService bool

	n, err := c.NetworkByID(nID)
	if err != nil {
		return err
	}

	skey := serviceKey{
		id:    svcID,
		ports: portConfigs(ingressPorts).String(),
	}

	var s *service
	for {
		c.Lock()
		var ok bool
		s, ok = c.serviceBindings[skey]
		if !ok {
			// Create a new service if we are seeing this service
			// for the first time.
			s = newService(svcName, svcID, ingressPorts, serviceAliases)
			c.serviceBindings[skey] = s
		}
		c.Unlock()
		s.Lock()
		if !s.deleted {
			// ok the object is good to be used
			break
		}
		s.Unlock()
	}
	logrus.Debugf("addServiceBinding from %s START for %s %s p:%p nid:%s skey:%v", getCaller(ctx), svcName, eID, s, nID, skey)
	defer s.Unlock()

	lb, ok := s.loadBalancers[nID]
	if !ok {
		// Create a new load balancer if we are seeing this
		// network attachment on the service for the first
		// time.
		fwMarkCtrMu.Lock()

		lb = &loadBalancer{
			vip:      vip,
			fwMark:   fwMarkCtr,
			backEnds: make(map[string]net.IP),
			service:  s,
		}

		fwMarkCtr++
		fwMarkCtrMu.Unlock()

		s.loadBalancers[nID] = lb
		addService = true
	}

	lb.backEnds[eID] = ip

	ok, entries := s.assignIPToEndpoint(ip.String(), eID)
	if !ok || entries > 1 {
		setStr, b := s.printIPToEndpoint(ip.String())
		logrus.Warnf("addServiceBinding %s possible trainsient state ok:%t entries:%d set:%t %s", eID, ok, entries, b, setStr)
	}

	// Add loadbalancer service and backend in all sandboxes in
	// the network only if vip is valid.
	if len(vip) != 0 {
		n.(*network).addLBBackend(ip, vip, lb.fwMark, ingressPorts)
	}

	newCtx := context.WithValue(ctx, callerCtxKey, "addServiceBinding")
	// Add the appropriate name resolutions
	c.addEndpointNameResolution(newCtx, svcName, svcID, nID, eID, containerName, vip, serviceAliases, taskAliases, ip, addService)

	logrus.Debugf("addServiceBinding from %s END for %s %s", getCaller(ctx), svcName, eID)

	return nil
}

func (c *controller) rmServiceBinding(ctx context.Context, svcName, svcID, nID, eID, containerName string, vip net.IP, ingressPorts []*PortConfig, serviceAliases []string, taskAliases []string, ip net.IP, deleteSvcRecords bool) error {

	var rmService bool

	n, err := c.NetworkByID(nID)
	if err != nil {
		return err
	}

	skey := serviceKey{
		id:    svcID,
		ports: portConfigs(ingressPorts).String(),
	}

	c.Lock()
	s, ok := c.serviceBindings[skey]
	c.Unlock()
	if !ok {
		logrus.Warnf("rmServiceBinding %s %s %s aborted c.serviceBindings[skey] !ok", getCaller(ctx), svcName, eID)
		return nil
	}

	s.Lock()
	defer s.Unlock()
	logrus.Debugf("rmServiceBinding from %s START for %s %s p:%p nid:%s sKey:%v deleteSvc:%t", getCaller(ctx), svcName, eID, s, nID, skey, deleteSvcRecords)
	lb, ok := s.loadBalancers[nID]
	if !ok {
		logrus.Warnf("rmServiceBinding %s %s %s aborted s.loadBalancers[nid] !ok", getCaller(ctx), svcName, eID)
		return nil
	}

	_, ok = lb.backEnds[eID]
	if !ok {
		logrus.Warnf("rmServiceBinding %s %s %s aborted lb.backEnds[eid] !ok", getCaller(ctx), svcName, eID)
		return nil
	}

	delete(lb.backEnds, eID)
	if len(lb.backEnds) == 0 {
		// All the backends for this service have been
		// removed. Time to remove the load balancer and also
		// remove the service entry in IPVS.
		rmService = true

		delete(s.loadBalancers, nID)
		logrus.Debugf("rmServiceBinding %s delete %s, p:%p in loadbalancers len:%d", eID, nID, lb, len(s.loadBalancers))
	}

	ok, entries := s.removeIPToEndpoint(ip.String(), eID)
	if !ok || entries > 0 {
		setStr, b := s.printIPToEndpoint(ip.String())
		logrus.Warnf("rmServiceBinding %s possible trainsient state ok:%t entries:%d set:%t %s", eID, ok, entries, b, setStr)
	}

	// Remove loadbalancer service(if needed) and backend in all
	// sandboxes in the network only if the vip is valid.
	if len(vip) != 0 && entries == 0 {
		n.(*network).rmLBBackend(ip, vip, lb.fwMark, ingressPorts, rmService)
	}

	// Delete the name resolutions
	if deleteSvcRecords {
		ctx := context.WithValue(ctx, callerCtxKey, "rmServiceBinding")
		c.deleteEndpointNameResolution(ctx, svcName, svcID, nID, eID, containerName, vip, serviceAliases, taskAliases, ip, rmService, entries > 0)
	}

	if len(s.loadBalancers) == 0 {
		// All loadbalancers for the service removed. Time to
		// remove the service itself.
		c.Lock()

		// Mark the object as deleted so that the add won't use it wrongly
		s.deleted = true
		// NOTE The delete from the serviceBindings map has to be the last operation else we are allowing a race between this service
		// that is getting deleted and a new service that will be created if the entry is not anymore there
		delete(c.serviceBindings, skey)
		c.Unlock()
	}

	logrus.Debugf("rmServiceBinding from %s END for %s %s", getCaller(ctx), svcName, eID)
	return nil
}
