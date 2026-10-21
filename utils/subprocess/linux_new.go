//go:build linux
// +build linux

// ^ syscall.SysProcAttr only has field Pdeathsig on Linux



package subprocess

import (
	"os/exec"
	"syscall"
)

func New(path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	return cmd
}
