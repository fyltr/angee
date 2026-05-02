//go:build !windows

package dev

import (
	"os/exec"
	"syscall"
)

// setSession puts the child in its own process group so SIGTERM can be
// delivered to the whole group via syscall.Kill(-pid, sig). Without
// Setsid, killing the parent leaves grandchildren orphaned (e.g. uv's
// inner python).
func setSession(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}

// signalGroup sends sig to the child's process group.
func signalGroup(r *running, sig syscall.Signal) {
	if r.cmd == nil || r.cmd.Process == nil {
		return
	}
	pid := r.cmd.Process.Pid
	// Negative PID delivers to the whole process group.
	_ = syscall.Kill(-pid, sig)
}
