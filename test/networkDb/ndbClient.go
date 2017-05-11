package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/libnetwork/networkdb"
)

var nDB *networkdb.NetworkDB
var localNodeName string

func checker(id int, ip, path, value string, c chan int) {
	for {
		resp, err := http.Get("http://localhost:" + ip + path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
			continue
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if strings.Contains(string(body), "could not get") {
			continue
		}
		fmt.Fprintf(os.Stderr, "%d ret is: %s", id, string(body))
		if err == nil && strings.Contains(string(body), value) {
			c <- id
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func writeAndPropagate(writer, path, key string, waitForNodes []string) {
	resp, err := http.Get("http://localhost:" + writer + path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	fmt.Fprintf(os.Stderr, "writer ret: %s", body)
	if err != nil || !strings.Contains(string(body), "OK") {
		fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
		// os.Exit(1)
	}

	nodes := len(waitForNodes)
	ch := make(chan int)
	for i, node := range waitForNodes {
		go checker(i, node, "/getentry?nid=test&tname=table_name&key="+key, "v", ch)
	}

	for {
		fmt.Fprintf(os.Stderr, "Missing %d nodes\n", nodes)
		id := <-ch
		fmt.Fprintf(os.Stderr, "%d done\n", id)
		nodes--
		if nodes == 0 {
			break
		}
	}

}

func main() {
	if len(os.Args) < 4 {
		log.Fatal("You need to specify the port and path")
	}
	operation := os.Args[1]
	key := os.Args[2]
	nodes := strings.Split(os.Args[3], ",")

	if operation == "write&propagate" {
		start := time.Now().UnixNano()
		writeAndPropagate(nodes[0], "/createentry?nid=test&tname=table_name&value=v&key="+key, key, nodes)
		opTime := time.Now().UnixNano() - start
		fmt.Fprintf(os.Stderr, "operation took: %dms\n", opTime/1000000)
	}

}
