//go:build !windows

package platform

import (
	"os/exec"
	"syscall"
)

// HideWindow 는 윈도우에서만 필요하다.
func HideWindow(c *exec.Cmd) {}

// Detach 는 부모가 끝나도 계속 도는 자식 프로세스로 만든다.
func Detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// AttachParentConsole 은 윈도우에서만 필요하다.
func AttachParentConsole() {}
