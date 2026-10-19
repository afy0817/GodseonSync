//go:build windows

package platform

import (
	"errors"
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
// 출력을 파일·파이프로 넘겼으면(`status > a.txt`, Git Bash) 그쪽을 그대로 쓴다 — 콘솔로 바꾸면 파일이 빈다.
func AttachParentConsole() {
	keepOut, keepErr := redirected(syscall.STD_OUTPUT_HANDLE), redirected(syscall.STD_ERROR_HANDLE)
	if keepOut && keepErr {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0)
	if r, _, _ := attach.Call(attachParentProcess); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		if !keepOut {
			os.Stdout = f
		}
		if !keepErr {
			os.Stderr = f
		}
	}
}

// AllowForeground 는 지금 앞으로 나올 권한이 있으면(사용자가 방금 누른 프로세스) 다른 프로세스에 넘긴다.
// 설정 창은 백그라운드 에이전트가 띄운 다른 프로세스라, 그대로면 윈도우가 앞으로 못 나오게 해서 다른 창 뒤에 뜬다.
// 권한이 없으면 아무 일도 안 한다.
func AllowForeground() {
	const asfwAny = ^uintptr(0) // ASFW_ANY
	syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow").Call(asfwAny)
}

// launchBlocked 는 윈도우 앱 컨트롤(스마트 앱 컨트롤, 그룹 정책)이 실행 파일을 막은 오류인지다.
// ERROR_ACCESS_DISABLED_BY_POLICY(1260), ERROR_SYSTEM_INTEGRITY_POLICY_VIOLATION(4551) 과 평판 차단 4556~4559·4580.
func launchBlocked(err error) bool {
	for _, code := range []syscall.Errno{1260, 4551, 4556, 4557, 4558, 4559, 4580} {
		if errors.Is(err, code) {
			return true
		}
	}
	return false
}

// redirected 는 표준 출력(또는 오류)이 파일이나 파이프로 넘겨졌는지다.
func redirected(std int) bool {
	h, err := syscall.GetStdHandle(std)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return false
	}
	t, _ := syscall.GetFileType(h)
	return t == syscall.FILE_TYPE_DISK || t == syscall.FILE_TYPE_PIPE
}
