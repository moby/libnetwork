package main

import (
	"os"
	"os/signal"

	psignal "github.com/docker/docker/pkg/signal"
	"golang.org/x/sys/unix"
)

func setupDumpStackTrap() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, unix.SIGUSR1)
	go func() {
		for range c {
			psignal.DumpStacks("")
		}
	}()
}
