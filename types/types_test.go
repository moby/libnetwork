package types

import (
	"net"
	"testing"

	_ "github.com/docker/libnetwork/netutils"
)

func TestTransportPortConv(t *testing.T) {
	sform := "tcp/23"
	tp := &TransportPort{Proto: TCP, Port: uint16(23)}

	if sform != tp.String() {
		t.Fatalf("String() method failed")
	}

	rc := new(TransportPort)
	if err := rc.FromString(sform); err != nil {
		t.Fatal(err)
	}
	if !tp.Equal(rc) {
		t.Fatalf("FromString() method failed")
	}
}

func TestTransportPortBindingConv(t *testing.T) {
	sform := "tcp/172.28.30.23:80/112.0.43.56:8001"
	pb := &PortBinding{
		Proto:    TCP,
		IP:       net.IPv4(172, 28, 30, 23),
		Port:     uint16(80),
		HostIP:   net.IPv4(112, 0, 43, 56),
		HostPort: uint16(8001),
	}

	rc := new(PortBinding)
	if err := rc.FromString(sform); err != nil {
		t.Fatal(err)
	}
	if !pb.Equal(rc) {
		t.Fatalf("FromString() method failed")
	}
}

func TestErrorConstructors(t *testing.T) {
	var err error

	err = BadRequestErrorf("Io ho %d uccello", 1)
	if err.Error() != "Io ho 1 uccello" {
		t.Fatal(err)
	}
	if _, ok := err.(BadRequestError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); ok {
		t.Fatal(err)
	}

	err = NotFoundErrorf("Can't find the %s", "keys")
	if err.Error() != "Can't find the keys" {
		t.Fatal(err)
	}
	if _, ok := err.(NotFoundError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); ok {
		t.Fatal(err)
	}

	err = ForbiddenErrorf("Can't open door %d", 2)
	if err.Error() != "Can't open door 2" {
		t.Fatal(err)
	}
	if _, ok := err.(ForbiddenError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); ok {
		t.Fatal(err)
	}

	err = NotImplementedErrorf("Functionality %s is not implemented", "x")
	if err.Error() != "Functionality x is not implemented" {
		t.Fatal(err)
	}
	if _, ok := err.(NotImplementedError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); ok {
		t.Fatal(err)
	}

	err = TimeoutErrorf("Process %s timed out", "abc")
	if err.Error() != "Process abc timed out" {
		t.Fatal(err)
	}
	if _, ok := err.(TimeoutError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); ok {
		t.Fatal(err)
	}

	err = NoServiceErrorf("Driver %s is not available", "mh")
	if err.Error() != "Driver mh is not available" {
		t.Fatal(err)
	}
	if _, ok := err.(NoServiceError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); ok {
		t.Fatal(err)
	}

	err = InternalErrorf("Not sure what happened")
	if err.Error() != "Not sure what happened" {
		t.Fatal(err)
	}
	if _, ok := err.(InternalError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); ok {
		t.Fatal(err)
	}

	err = InternalMaskableErrorf("Minor issue, it can be ignored")
	if err.Error() != "Minor issue, it can be ignored" {
		t.Fatal(err)
	}
	if _, ok := err.(InternalError); !ok {
		t.Fatal(err)
	}
	if _, ok := err.(MaskableError); !ok {
		t.Fatal(err)
	}
}
