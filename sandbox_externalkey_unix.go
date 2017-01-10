// +build linux freebsd

package libnetwork

import (
	"encoding/json"
	"net"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/libnetwork/types"
)

const udsBase = "/run/docker/libnetwork/"
const success = "success"

type setKeyData struct {
	ContainerID string
	Key         string
}

func (c *controller) startExternalKeyListener() error {
	if err := os.MkdirAll(udsBase, 0600); err != nil {
		return err
	}
	uds := udsBase + c.id + ".sock"
	l, err := net.Listen("unix", uds)
	if err != nil {
		return err
	}
	if err := os.Chmod(uds, 0600); err != nil {
		l.Close()
		return err
	}
	c.Lock()
	c.extKeyListener = l
	c.Unlock()

	go c.acceptClientConnections(uds, l)
	return nil
}

func (c *controller) acceptClientConnections(sock string, l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			if _, err1 := os.Stat(sock); os.IsNotExist(err1) {
				logrus.Debugf("Unix socket %s doesn't exist. cannot accept client connections", sock)
				return
			}
			logrus.Errorf("Error accepting connection %v", err)
			continue
		}
		go func() {
			defer conn.Close()

			err := c.processExternalKey(conn)
			ret := success
			if err != nil {
				ret = err.Error()
			}

			_, err = conn.Write([]byte(ret))
			if err != nil {
				logrus.Errorf("Error returning to the client %v", err)
			}
		}()
	}
}

func (c *controller) processExternalKey(conn net.Conn) error {
	buf := make([]byte, 1280)
	nr, err := conn.Read(buf)
	if err != nil {
		return err
	}
	var s setKeyData
	if err = json.Unmarshal(buf[0:nr], &s); err != nil {
		return err
	}

	var sandbox Sandbox
	search := SandboxContainerWalker(&sandbox, s.ContainerID)
	c.WalkSandboxes(search)
	if sandbox == nil {
		return types.BadRequestErrorf("no sandbox present for %s", s.ContainerID)
	}

	return sandbox.SetKey(s.Key)
}

func (c *controller) stopExternalKeyListener() {
	c.extKeyListener.Close()
}
