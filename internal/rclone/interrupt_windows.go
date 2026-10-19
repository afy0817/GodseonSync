//go:build windows

package rclone

import "os"

// 윈도우는 다른 프로세스에 Ctrl+C 를 보내기 어려워 바로 끝낸다 (다음 실행 때 --recover 로 이어 간다).
func interrupt(p *os.Process) error { return p.Kill() }
