//go:build !linux
// +build !linux



package subprocess

import "os/exec"

func New(path string, args ...string) *exec.Cmd {
	return exec.Command(path, args...)
}
