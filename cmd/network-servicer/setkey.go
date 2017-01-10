package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"

	"github.com/opencontainers/runc/libcontainer/configs"
)

const udsBase = "/run/docker/libnetwork/"
const success = "success"

type setKeyData struct {
	ContainerID string
	Key         string
}

func processSetKey(containerID, controllerID string) (int, error) {
	// We expect configs.HookState as a json string in <stdin>
	stateBuf, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return 2, err
	}
	var state configs.HookState
	if err := json.Unmarshal(stateBuf, &state); err != nil {
		return 3, err
	}

	err = SetExternalKey(controllerID, containerID, fmt.Sprintf("/proc/%d/ns/net", state.Pid))
	if err != nil {
		return 4, err
	}

	return 0, nil
}

// SetExternalKey provides a convenient way to set an External key to a sandbox
func SetExternalKey(controllerID string, containerID string, key string) error {
	keyData := setKeyData{
		ContainerID: containerID,
		Key:         key}

	c, err := net.Dial("unix", udsBase+controllerID+".sock")
	if err != nil {
		return err
	}
	defer c.Close()

	if err = sendKey(c, keyData); err != nil {
		return fmt.Errorf("sendKey failed with : %v", err)
	}
	return processReturn(c)
}

func sendKey(c net.Conn, data setKeyData) error {
	var err error
	defer func() {
		if err != nil {
			c.Close()
		}
	}()

	var b []byte
	if b, err = json.Marshal(data); err != nil {
		return err
	}

	_, err = c.Write(b)
	return err
}

func processReturn(r io.Reader) error {
	buf := make([]byte, 1024)
	n, err := r.Read(buf[:])
	if err != nil {
		return fmt.Errorf("failed to read buf in processReturn : %v", err)
	}
	if string(buf[0:n]) != success {
		return fmt.Errorf(string(buf[0:n]))
	}
	return nil
}
