//go:build windows

package platform

import (
	"os"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// HideWindow 는 자식 프로세스의 콘솔 창이 뜨지 않게 한다.
func HideWindow(c *exec.Cmd) {
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.HideWindow = true
	c.SysProcAttr.CreationFlags |= createNoWindow
}

// Detach 는 부모가 끝나도 계속 도는 자식 프로세스로 만든다.
func Detach(c *exec.Cmd) {
	HideWindow(c)
	c.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// AttachParentConsole 은 창 없는(GUI) 실행 파일을 명령 프롬프트에서 실행했을 때 출력이 보이게 한다.
func AttachParentConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0)
	if r, _, _ := attach.Call(attachParentProcess); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = f
		os.Stderr = f
	}
}
