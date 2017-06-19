package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var localNodeName string

func checker(id int, ip, path, value string, c chan int) {
	for {
		body, err := httpGet(ip, path)
		if strings.Contains(string(body), "could not get") {
			continue
		}
		fmt.Fprintf(os.Stderr, "%d ret is: %s", id, string(body))
		if err == nil && strings.Contains(string(body), value) {
			c <- id
			return
		}
		// time.Sleep(100 * time.Millisecond)
	}
}

func writeKey(port, path string) error {
	body, err := httpGet(port, path)

	// fmt.Fprintf(os.Stderr, "writer ret: %s", body)
	if err != nil || !strings.Contains(string(body), "OK") {
		fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
		return fmt.Errorf("Write error %s", err)
	}
	return nil
}

func httpGet(port, path string) ([]byte, error) {
	resp, err := http.Get("http://localhost:" + port + path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	return body, err
}

func joinNetwork(port, network string) error {
	body, err := httpGet(port, "/joinnetwork?nid="+network)

	if err != nil || !strings.Contains(string(body), "OK") {
		fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
		return fmt.Errorf("joinNetwork error %s", err)
	}
	return nil
}

func leaveNetwork(port, network string) error {
	body, err := httpGet(port, "/leavenetwork?nid="+network)

	if err != nil || !strings.Contains(string(body), "OK") {
		fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
		return fmt.Errorf("leaveNetwork error %s", err)
	}
	return nil
}

func writeAndPropagate(writer, path, key string, waitForNodes []string) {
	writeKey(writer, path)

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

func doWriteDelete(ctx context.Context, port, key string, doneCh chan int) {
	x := 0
	createPath := "/createentry?nid=test&tname=table_name&value=v&key="
	deletePath := "/deleteentry?nid=test&tname=table_name&key="
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "Exiting after having written %s keys\n", strconv.Itoa(x))
			doneCh <- x
			return
		default:
			k := key + "-" + strconv.Itoa(x)
			// write key
			// fmt.Fprintf(os.Stderr, "Write %s\n", createPath+k)
			err := writeKey(port, createPath+k)
			if err != nil {
				//error
			}
			// delete key
			// fmt.Fprintf(os.Stderr, "Delete %s\n", deletePath+k)
			err = writeKey(port, deletePath+k)
			if err != nil {
				//error
			}
			x++
			// if x == 100 {
			// 	doneCh <- x
			// 	return
			// }
		}
		// time.Sleep(200 * time.Millisecond)
	}
}

func doWriteDeleteLeaveJoin(ctx context.Context, port, key string, doneCh chan int) {
	x := 0
	createPath := "/createentry?nid=test&tname=table_name&value=v&key="
	deletePath := "/deleteentry?nid=test&tname=table_name&key="

	fmt.Fprintf(os.Stderr, "%s Started\n", key)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "Exiting after having written %s keys\n", strconv.Itoa(x))
			doneCh <- x
			return
		default:
			k := key + "-" + strconv.Itoa(x)
			// write key
			// fmt.Fprintf(os.Stderr, "Write %s\n", createPath+k)
			err := writeKey(port, createPath+k)
			if err != nil {
				//error
			}
			// delete key
			// fmt.Fprintf(os.Stderr, "Delete %s\n", deletePath+k)
			err = writeKey(port, deletePath+k)
			if err != nil {
				//error
			}
			x++
			time.Sleep(100 * time.Millisecond)
			// leave network
			fmt.Fprintf(os.Stderr, "%s Leave network\n", key)
			err = leaveNetwork(port, "test")
			if err != nil {
				//error
			}
			time.Sleep(100 * time.Millisecond)
			// join network
			fmt.Fprintf(os.Stderr, "%s Join network\n", key)
			err = joinNetwork(port, "test")
			if err != nil {
				//error
			}
		}
		// time.Sleep(200 * time.Millisecond)
	}
}

func writeAndDelete(writerList []string, keyBase string) {
	workers := len(writerList)
	doneCh := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())

	// start the write in parallel
	for _, w := range writerList {
		key := keyBase + w
		fmt.Fprintf(os.Stderr, "Spawn worker: %s\n", w)
		go doWriteDelete(ctx, w, key, doneCh)
	}
	time.Sleep(10 * time.Second)
	cancel()
	for workers > 0 {
		fmt.Fprintf(os.Stderr, "Remains: %d workers\n", workers)
		<-doneCh
		workers--
	}

	// Stop when stable
	stableResult := 3
	start := time.Now().UnixNano()
	for {
		time.Sleep(2 * time.Second)
		fmt.Fprintf(os.Stderr, "Checking node tables\n")
		var equal int
		var prev []byte
		for i, w := range writerList {
			path := "/gettable?nid=test&tname=table_name"
			body, err := httpGet(string(w), path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
				return
			}
			_, line, _ := bufio.ScanLines(body, false)
			fmt.Fprintf(os.Stderr, "%s writer ret: %s\n", w, line)

			if i > 0 {
				if bytes.Equal(prev, body) {
					equal++
				} else {
					equal = 0
					stableResult = 3
				}
			}
			prev = body
			if equal == len(writerList)-1 {
				stableResult--
				if stableResult == 0 {
					opTime := time.Now().UnixNano() - start
					fmt.Fprintf(os.Stderr, "the output is stable after: %dms\n", opTime/1000000)
					return
				}
			}
		}
	}
}

func writeAndDeleteLeaveJoin(writerList []string, keyBase string) {
	workers := len(writerList)
	doneCh := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())

	// start the write in parallel
	for _, w := range writerList {
		key := keyBase + w
		fmt.Fprintf(os.Stderr, "Spawn worker: %s\n", w)
		go doWriteDeleteLeaveJoin(ctx, w, key, doneCh)
	}
	time.Sleep(5 * time.Second)
	cancel()
	for workers > 0 {
		fmt.Fprintf(os.Stderr, "Remains: %d workers\n", workers)
		<-doneCh
		workers--
	}

	// Stop when stable
	stableResult := 3
	start := time.Now().UnixNano()
	for {
		time.Sleep(2 * time.Second)
		fmt.Fprintf(os.Stderr, "Checking node tables\n")
		var equal int
		var prev []byte
		for i, w := range writerList {
			path := "/gettable?nid=test&tname=table_name"
			body, err := httpGet(string(w), path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "there was an error: %s\n", err)
				return
			}
			_, line, _ := bufio.ScanLines(body, false)
			fmt.Fprintf(os.Stderr, "%s writer ret: %s\n", w, line)

			if i > 0 {
				if bytes.Equal(prev, body) {
					equal++
				} else {
					equal = 0
					stableResult = 3
				}
			}
			prev = body
			if equal == len(writerList)-1 {
				stableResult--
				if stableResult == 0 {
					opTime := time.Now().UnixNano() - start
					fmt.Fprintf(os.Stderr, "the output is stable after: %dms\n", opTime/1000000)
					return
				}
			}
		}
	}

}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("You need to specify the port and path")
	}
	operation := os.Args[1]
	nodes := strings.Split(os.Args[2], ",")
	key := "testKey-"
	if len(os.Args) > 3 {
		key = os.Args[3]
	}

	switch operation {
	case "write-propagate":
		start := time.Now().UnixNano()
		writeAndPropagate(nodes[0], "/createentry?nid=test&tname=table_name&value=v&key="+key, key, nodes)
		opTime := time.Now().UnixNano() - start
		fmt.Fprintf(os.Stderr, "operation took: %dms\n", opTime/1000000)
	case "write-delete":
		writeAndDelete(nodes, "testKey-")
	case "write-delete-leave-join":
		writeAndDeleteLeaveJoin(nodes, "testKey-")
	default:
		log.Fatal("Operations: write-propagate, write-delete")
	}

}
