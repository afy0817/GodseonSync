//go:build windows

package platform

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procWTSQuerySessionInformation = windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSQuerySessionInformationW")

const wtsSessionInfo = 24 // WTS_INFO_CLASS 의 WTSSessionInfo

// wtsInfo 는 WTSINFOW 다. 시각은 FILETIME 값(1601년부터 100나노초).
type wtsInfo struct {
	State     uint32
	SessionID uint32
	_         [6]uint32            // 주고받은 바이트·프레임 수
	_         [32 + 17 + 21]uint16 // WinStationName, Domain, UserName
	Connect   int64
	Disconn   int64
	LastInput int64
	Logon     int64
	Current   int64
}

// loginTime 은 이 세션에 로그인한 시각이다. 모르면 0.
func loginTime() time.Time {
	var p *wtsInfo
	var n uint32
	const currentServer, currentSession = 0, 0xFFFFFFFF
	r, _, _ := procWTSQuerySessionInformation.Call(currentServer, currentSession, wtsSessionInfo,
		uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&n)))
	if r == 0 || p == nil {
		return time.Time{}
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(p)))
	if n < uint32(unsafe.Sizeof(*p)) || p.Logon <= 0 {
		return time.Time{}
	}
	// 구조를 잘못 읽었으면 '지금 시각' 칸이 지금과 다르다 → 모르는 것으로 둔다
	now := fileTime(p.Current)
	if d := time.Since(now); d > 5*time.Minute || d < -5*time.Minute {
		return time.Time{}
	}
	return fileTime(p.Logon)
}

func fileTime(v int64) time.Time {
	ft := windows.Filetime{LowDateTime: uint32(v), HighDateTime: uint32(v >> 32)}
	return time.Unix(0, ft.Nanoseconds())
}
