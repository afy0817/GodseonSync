//go:build windows

package platform

import (
	"fmt"
	"os"
	"syscall"
	"testing"
)

// 스마트 앱 컨트롤이 막으면 exec 는 *os.PathError{Op: "fork/exec", Err: 4551} 을 돌려준다 (2026-10-20 실제 로그).
func TestLaunchBlocked(t *testing.T) {
	blocked := &os.PathError{Op: "fork/exec", Path: `C:\GodseonSync.exe`, Err: syscall.Errno(4551)}
	if !LaunchBlocked(blocked) || !LaunchBlocked(fmt.Errorf("감쌈: %w", blocked)) {
		t.Fatal("앱 컨트롤 차단을 못 알아봄")
	}
	if LaunchBlocked(&os.PathError{Op: "fork/exec", Err: syscall.ERROR_FILE_NOT_FOUND}) || LaunchBlocked(nil) {
		t.Fatal("다른 오류를 차단으로 봄")
	}
}
