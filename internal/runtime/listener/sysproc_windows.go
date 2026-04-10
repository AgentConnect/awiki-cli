//go:build windows
// +build windows

package listener

import "os/exec"

// setSysProcAttr is a no-op on Windows. The listener supervisor is not yet
// supported as a background service on this platform.
func setSysProcAttr(cmd *exec.Cmd) {
	// Intentionally empty.
	_ = cmd
}
