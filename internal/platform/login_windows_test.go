//go:build windows

package platform

import (
	"testing"
	"time"
	"unsafe"
)

// WTSINFOW 는 216바이트, LogonTime 은 200바이트 위치다 (x64). 어긋나면 엉뚱한 칸을 로그인 시각으로 읽는다.
func TestWTSInfoLayout(t *testing.T) {
	var w wtsInfo
	if s := unsafe.Sizeof(w); s != 216 {
		t.Fatalf("크기 %d, 216 이어야 함", s)
	}
	if o := unsafe.Offsetof(w.Logon); o != 200 {
		t.Fatalf("Logon 위치 %d, 200 이어야 함", o)
	}
}

func TestLoginTimeBeforeNow(t *testing.T) {
	lt := loginTime()
	if lt.IsZero() {
		t.Skip("이 세션의 로그인 시각을 모름 (서비스·CI 세션)")
	}
	if lt.After(time.Now().Add(time.Minute)) {
		t.Fatalf("로그인 시각이 미래: %v", lt)
	}
}
