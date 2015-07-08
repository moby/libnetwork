package namespace

import (
	"reflect"
	"testing"

	"github.com/docker/libnetwork/netlabel"
	_ "github.com/docker/libnetwork/netutils"
	"github.com/docker/libnetwork/options"
)

func TestDriver(t *testing.T) {
	d := &driver{}

	if d.Type() != networkType {
		t.Fatalf("Unexpected network type returned by driver")
	}

	err := d.CreateNetwork("first", nil)
	if err != nil {
		t.Fatal(err)
	}

	if d.network != "first" {
		t.Fatalf("Unexpected network id stored")
	}
}

func TestParseNamespaceOptions(t *testing.T) {
	testOptions := options.Generic{
		netlabel.GenericNamespaceOptions: options.Generic{
			"ContainerID":     "12345",
			"CustomNamespace": "/var/run/netns/alec",
		},
	}
	expectedConfig := &endpointConfig{
		ContainerID:     "12345",
		CustomNamespace: "/var/run/netns/alec",
	}

	config, err := parseNamespaceOptions(testOptions)
	if err != nil {
		t.Fatalf("error in TestParseNamespaceOptions: %v", err)
	}

	if !reflect.DeepEqual(config, expectedConfig) {
		t.Fatalf("expected parseConfig to return %+v, got %+v instead", expectedConfig, config)
	}
}
