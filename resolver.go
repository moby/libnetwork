package libnetwork

import (
	"fmt"
	"net"
	"strings"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/libnetwork/iptables"
	"github.com/miekg/dns"
)

// Resolver represents the embedded DNS server in Docker. It listens
// on an IP in the loopback range in the container's namespace
type Resolver interface {
	// Start starts the name server for the container
	Start() error
	// Stop stops the name server for the container. Stopped resolver
	// can be reused after running the SetupFunc again.
	Stop()
	// SetupFunc() provides the setup function that should be run
	// in the container's network namespace.
	SetupFunc() func()
	// NameServer() returns the IP of the DNS resolver for the
	// containers.
	NameServer() string
	// To configure external name servers the resolver should use
	SetExtServers([]string)
	// ResolverOptions returns resolv.conf options that should be set
	ResolverOptions() []string
}

const (
	resolverIP    = "127.0.0.11"
	dnsPort       = "53"
	ptrIPv4domain = ".in-addr.arpa."
	ptrIPv6domain = ".ip6.arpa."
	respTTL       = 1800
)

// resolver implements the Resolver interface
type resolver struct {
	sb     *sandbox
	extDNS []string
	server *dns.Server
	conn   *net.UDPConn
	err    error
}

// NewResolver creates a new instance of the Resolver
func NewResolver(sb *sandbox) Resolver {
	return &resolver{
		sb:  sb,
		err: fmt.Errorf("setup not done yet"),
	}
}

func (r *resolver) SetupFunc() func() {
	return (func() {
		var err error

		addr := &net.UDPAddr{
			IP: net.ParseIP(resolverIP),
		}

		r.conn, err = net.ListenUDP("udp", addr)
		if err != nil {
			r.err = fmt.Errorf("error in opening name server socket %v", err)
			return
		}
		laddr := r.conn.LocalAddr()
		_, ipPort, _ := net.SplitHostPort(laddr.String())

		rules := [][]string{
			{"-t", "nat", "-A", "OUTPUT", "-d", resolverIP, "-p", "udp", "--dport", dnsPort, "-j", "DNAT", "--to-destination", laddr.String()},
			{"-t", "nat", "-A", "POSTROUTING", "-s", resolverIP, "-p", "udp", "--sport", ipPort, "-j", "SNAT", "--to-source", ":" + dnsPort},
		}

		for _, rule := range rules {
			r.err = iptables.RawCombinedOutput(rule...)
			if r.err != nil {
				return
			}
		}
		r.err = nil
	})
}

func (r *resolver) Start() error {
	// make sure the resolver has been setup before starting
	if r.err != nil {
		return r.err
	}
	errCh := make(chan error)

	s := &dns.Server{Handler: r, PacketConn: r.conn,
		NotifyStartedFunc: func() {
			if r.conn == nil {
				errCh <- fmt.Errorf("listener socket is not open")
				return
			}
			file, err := r.conn.File()
			if err != nil {
				errCh <- err
				return
			}
			// miekg/dns library sets the socket option IP_PKTINFO which resets
			// the non-blocking mode. Set it back here so that read from socket
			// blocks on the net poller. Otherwise it would block on Read system
			// call costing a OS thread per socket.
			errCh <- syscall.SetNonblock(int(file.Fd()), true)
		}}
	r.server = s
	go func() {
		s.ActivateAndServe()
	}()
	return <-errCh
}

func (r *resolver) Stop() {
	if r.server != nil {
		r.server.Shutdown()
	}
	r.conn = nil
	r.err = fmt.Errorf("setup not done yet")
}

func (r *resolver) SetExtServers(dns []string) {
	r.extDNS = dns
}

func (r *resolver) NameServer() string {
	return resolverIP
}

func (r *resolver) ResolverOptions() []string {
	return []string{"ndots:0"}
}

func (r *resolver) handleIPv4Query(name string, query *dns.Msg) (*dns.Msg, error) {
	addr := r.sb.ResolveName(name)
	if addr == nil {
		return nil, nil
	}

	log.Debugf("Lookup for %s: IP %s", name, addr.String())

	resp := new(dns.Msg)
	resp.SetReply(query)

	rr := new(dns.A)
	rr.Hdr = dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: respTTL}
	rr.A = addr
	resp.Answer = append(resp.Answer, rr)
	return resp, nil
}

func (r *resolver) handlePTRQuery(ptr string, query *dns.Msg) (*dns.Msg, error) {
	parts := []string{}

	if strings.HasSuffix(ptr, ptrIPv4domain) {
		parts = strings.Split(ptr, ptrIPv4domain)
	} else if strings.HasSuffix(ptr, ptrIPv6domain) {
		parts = strings.Split(ptr, ptrIPv6domain)
	} else {
		return nil, fmt.Errorf("invalid PTR query, %v", ptr)
	}

	host := r.sb.ResolveIP(parts[0])
	if len(host) == 0 {
		return nil, nil
	}

	log.Debugf("Lookup for IP %s: name %s", parts[0], host)
	fqdn := dns.Fqdn(host)

	resp := new(dns.Msg)
	resp.SetReply(query)

	rr := new(dns.PTR)
	rr.Hdr = dns.RR_Header{Name: ptr, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: respTTL}
	rr.Ptr = fqdn
	resp.Answer = append(resp.Answer, rr)
	return resp, nil
}

func (r *resolver) ServeDNS(w dns.ResponseWriter, query *dns.Msg) {
	var (
		resp *dns.Msg
		err  error
	)

	name := query.Question[0].Name
	if query.Question[0].Qtype == dns.TypeA {
		resp, err = r.handleIPv4Query(name, query)
	} else if query.Question[0].Qtype == dns.TypePTR {
		resp, err = r.handlePTRQuery(name, query)
	}

	if err != nil {
		log.Error(err)
		return
	}

	if resp == nil {
		if len(r.extDNS) == 0 {
			return
		}
		log.Debugf("Querying ext dns %s for %s[%d]", r.extDNS[0], name, query.Question[0].Qtype)

		c := &dns.Client{Net: "udp"}
		addr := fmt.Sprintf("%s:%d", r.extDNS[0], 53)

		// TODO: iterate over avilable servers in case of error
		resp, _, err = c.Exchange(query, addr)
		if err != nil {
			log.Errorf("external resolution failed, %s", err)
			return
		}
	}

	err = w.WriteMsg(resp)
	if err != nil {
		log.Errorf("error writing resolver resp, %s", err)
	}
}
