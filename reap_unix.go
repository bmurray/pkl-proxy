//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// reapChildren handles orphaned child processes when running as PID 1 (e.g. in Docker).
// It listens for SIGCHLD and reaps zombie processes to prevent them from accumulating.
func reapChildren() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGCHLD)
	for range sig {
		for {
			var status syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if pid <= 0 || err != nil {
				break
			}
		}
	}
}
