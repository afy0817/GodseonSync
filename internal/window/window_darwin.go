//go:build darwin

package window

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
void gsRunWindow(const char *url, const char *title, int width, int height);
void gsFocusWindow(void);
void gsCloseWindow(void);
*/
import "C"

import (
	"runtime"
	"unsafe"
)

func init() { runtime.LockOSThread() } // Cocoa 는 메인 스레드에서만 돈다

// Supported 는 전용 창을 쓸 수 있는지 알려 준다.
func Supported() bool { return true }

// Run 은 창을 띄우고 창이 닫힐 때까지 돌아오지 않는다 (닫히면 프로세스가 끝난다).
func Run(url string) error {
	cu, ct := C.CString(url), C.CString(Title)
	defer C.free(unsafe.Pointer(cu))
	defer C.free(unsafe.Pointer(ct))
	C.gsRunWindow(cu, ct, C.int(Width), C.int(Height))
	return nil
}

// Focus 는 창을 맨 앞으로 가져온다 (다른 스레드에서 불러도 된다).
func Focus() { C.gsFocusWindow() }

// Close 는 창을 닫고 프로세스를 끝낸다.
func Close() { C.gsCloseWindow() }
