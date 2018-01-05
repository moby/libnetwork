package networkdb

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddNetworkNode(t *testing.T) {
	n := &NetworkDB{config: &Config{NodeID: "node-0"}, networkNodes: make(map[string]map[string]struct{})}
	for i := 0; i < 2000; i++ {
		n.addNetworkNode("network", "node-"+strconv.Itoa(i%1000))
	}
	assert.Equal(t, 1000, len(n.networkNodes["network"]))
	for i := 0; i < 2000; i++ {
		n.addNetworkNode("network"+strconv.Itoa(i%1000), "node-"+strconv.Itoa(i))
	}
	for i := 0; i < 1000; i++ {
		assert.Equal(t, 2, len(n.networkNodes["network"+strconv.Itoa(i%1000)]))
	}
}

func TestDeleteNetworkNode(t *testing.T) {
	n := &NetworkDB{config: &Config{NodeID: "node-0"}, networkNodes: make(map[string]map[string]struct{})}
	for i := 0; i < 1000; i++ {
		n.addNetworkNode("network", "node-"+strconv.Itoa(i%1000))
	}
	assert.Equal(t, 1000, len(n.networkNodes["network"]))
	for i := 0; i < 2000; i++ {
		n.deleteNetworkNode("network", "node-"+strconv.Itoa(i%1000))
	}
	assert.Equal(t, 0, len(n.networkNodes["network"]))
	for i := 0; i < 2000; i++ {
		n.addNetworkNode("network"+strconv.Itoa(i%1000), "node-"+strconv.Itoa(i))
	}
	for i := 0; i < 1000; i++ {
		assert.Equal(t, 2, len(n.networkNodes["network"+strconv.Itoa(i%1000)]))
		n.deleteNetworkNode("network"+strconv.Itoa(i%1000), "node-"+strconv.Itoa(i))
		assert.Equal(t, 1, len(n.networkNodes["network"+strconv.Itoa(i%1000)]))
	}
	for i := 1000; i < 2000; i++ {
		n.deleteNetworkNode("network"+strconv.Itoa(i%1000), "node-"+strconv.Itoa(i))
		assert.Equal(t, 0, len(n.networkNodes["network"+strconv.Itoa(i%1000)]))
	}
}

func TestRandomNodes(t *testing.T) {
	n := &NetworkDB{config: &Config{NodeID: "node-0"}}
	nodes := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		nodes["node-"+strconv.Itoa(i)] = struct{}{}
	}
	nodeHit := make(map[string]int)
	for i := 0; i < 5000; i++ {
		chosen := n.mRandomNodes(3, nodes)
		for _, c := range chosen {
			if c == "node-0" {
				t.Fatal("should never hit itself")
			}
			nodeHit[c]++
		}
	}

	// check results
	var min, max int
	for node, hit := range nodeHit {
		if min == 0 {
			min = hit
		}
		if hit == 0 && node != "node-0" {
			t.Fatal("node never hit")
		}
		if hit > max {
			max = hit
		}
		if hit < min {
			min = hit
		}
	}
	assert.NotEqual(t, 0, min)
}

func BenchmarkAddNetworkNode(b *testing.B) {
	n := &NetworkDB{config: &Config{NodeID: "node-0"}, networkNodes: make(map[string]map[string]struct{})}
	for i := 0; i < b.N; i++ {
		n.addNetworkNode("network", "node-"+strconv.Itoa(i%1000))
	}
}

func BenchmarkDeleteNetworkNode(b *testing.B) {
	n := &NetworkDB{config: &Config{NodeID: "node-0"}, networkNodes: make(map[string]map[string]struct{})}
	nodes := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		name := "node-" + strconv.Itoa(i)
		n.addNetworkNode("network", name)
		nodes = append(nodes, name)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.deleteNetworkNode("network", nodes[i%1000])
	}
}

func BenchmarkRandomNodes(b *testing.B) {
	n := &NetworkDB{config: &Config{NodeID: "node-0"}}
	nodes := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		nodes["node-"+strconv.Itoa(i)] = struct{}{}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.mRandomNodes(3, nodes)
	}
}
