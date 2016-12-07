// +build !windows

package libnetwork

import (
	"fmt"
	"os"
	"os/exec"
)

func (r *resolver) setupIPTable() error {
	if r.err != nil {
		return r.err
	}

	execPath, err := exec.LookPath(servicerCommandName)
	if err != nil {
		return fmt.Errorf("failed to find %s binary: %v", servicerCommandName, err)
	}

	args := []string{
		execPath,
		"resolver",
		"-path", r.resolverKey,
		"-lip", r.conn.LocalAddr().String(),
		"-ltcp", r.tcpListen.Addr().String(),
	}

	cmd := &exec.Cmd{
		Path:   execPath,
		Args:   args,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resolver failed: %v", err)
	}

	return nil
}
