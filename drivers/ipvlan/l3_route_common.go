package ipvlan

import (
	"net"
)

type ribCache struct {
	BgpTable map[string]*ribLocal
}

// Unmarshalled BGP update binding for simplicity
type ribLocal struct {
	BgpPrefix    *net.IPNet
	OriginatorIP net.IP
	NextHop      net.IP
	Age          int
	Best         bool
	IsWithdraw   bool
	IsHostRoute  bool
	IsLocal      bool
	AsPath       string
	RT           string
}

type ribTest struct {
	BgpTable map[string]*ribLocal
}
