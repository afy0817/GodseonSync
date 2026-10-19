//go:build windows

package platform

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// comObject 는 COM·WinRT 객체다 (첫 칸이 메서드 표). 폴더 선택 창과 알림이 cgo 없이 OS API 를 부를 때 쓴다.
type comObject struct{ vtbl *[32]uintptr }

// call 은 메서드 표의 n 번째 메서드를 부르고 HRESULT 를 돌려준다.
// 인자는 uintptr 로 바꾼 포인터라 표시가 없으면 부르는 사이 스택이 옮겨져 OS 가 옛 주소에 쓸 수 있다 → 힙에 두게 한다.
//
//go:uintptrescapes
func (o *comObject) call(n int, args ...uintptr) uint32 {
	r, _, _ := syscall.SyscallN(o.vtbl[n], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return uint32(r)
}

// query 는 다른 인터페이스로 바꾼다 (IUnknown::QueryInterface).
func (o *comObject) query(iid *windows.GUID) (*comObject, error) {
	var out *comObject
	if hr := o.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); hr != 0 {
		return nil, fmt.Errorf("QueryInterface %v: 0x%08x", iid, hr)
	}
	return out, nil
}

// release 는 다 쓴 객체를 놓는다 (IUnknown::Release).
func (o *comObject) release() {
	if o != nil {
		o.call(2)
	}
}
