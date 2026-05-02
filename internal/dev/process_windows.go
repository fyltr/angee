//go:build windows

package dev

import (
	"os/exec"
	"syscall"
)

const cREATE_NEW_PROCESS_GROUP = 0x00000200

// setSession puts the child in its own console process group so the
// orchestrator can deliver CTRL_BREAK_EVENT (the closest Windows cousin
// to SIGTERM-to-group) to its descendants without affecting itself.
func setSession(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= cREATE_NEW_PROCESS_GROUP
}

// signalGroup sends a CTRL_BREAK_EVENT to the child's process group on
// SIGTERM/SIGINT and falls back to Process.Kill on SIGKILL.
func signalGroup(r *running, sig syscall.Signal) {
	if r.cmd == nil || r.cmd.Process == nil {
		return
	}
	switch sig {
	case syscall.SIGKILL:
		_ = r.cmd.Process.Kill()
	default:
		// Best-effort: ask the process group to break.
		dll, err := syscall.LoadDLL("kernel32.dll")
		if err != nil {
			_ = r.cmd.Process.Kill()
			return
		}
		proc, err := dll.FindProc("GenerateConsoleCtrlEvent")
		if err != nil {
			_ = r.cmd.Process.Kill()
			return
		}
		// 1 = CTRL_BREAK_EVENT; second arg is the process group id (PID).
		_, _, _ = proc.Call(1, uintptr(r.cmd.Process.Pid))
	}
}
