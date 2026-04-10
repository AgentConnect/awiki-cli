//go:build !windows
// +build !windows

package listener

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr configures the child process attributes for Unix-like systems.
// It is a no-op on Windows (see sysproc_windows.go).
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
