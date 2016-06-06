//+build !windows

package portmapper

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
)

type proxyRestored struct {
	process *os.Process
}

func newProxyRestored(proto string, hostIP net.IP, hostPort int, containerIP net.IP, containerPort int) (userlandProxy, error) {
	str := fmt.Sprintf("%s/%s/%d/%s/%d", proto, hostIP.String(), hostPort, containerIP.String(), containerPort)
	pid, err := getProcessPid(str)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("restore old userland proxy 'pid=%d' for %s ", pid, str)
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	return &proxyRestored{process: process}, nil
}

func (p *proxyRestored) Start() error {
	return nil
}

func (p *proxyRestored) Stop() error {
	p.process.Kill()
	return nil
}

func getProcessPid(s string) (int, error) {
	psArgs := "-ef"
	output, err := exec.Command("ps", strings.Split(psArgs, " ")...).Output()
	if err != nil {
		return -1, fmt.Errorf("Error running ps: %v", err)
	}
	lines := strings.Split(string(output), "\n")
	pidIndex := -1
	cmdIndex := -1
	for i, name := range strings.Fields(lines[0]) {
		if name == "PID" {
			pidIndex = i
		}
		if name == "CMD" {
			cmdIndex = i
		}
	}
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		fields := strings.Fields(line)
		if strings.Compare(fields[cmdIndex], userlandProxyCommandName) == 0 {
			str := fmt.Sprintf("%s/%s/%s/%s/%s", fields[cmdIndex+2], fields[cmdIndex+4], fields[cmdIndex+6], fields[cmdIndex+8], fields[cmdIndex+10])
			if strings.Compare(str, s) == 0 {
				p, err := strconv.Atoi(fields[pidIndex])
				if err != nil {
					return -1, fmt.Errorf("Unexpected pid '%s': %s", fields[pidIndex], err)
				}
				return p, nil
			}
		}
	}
	return -1, fmt.Errorf("no such proxy")
}
