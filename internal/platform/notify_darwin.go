//go:build darwin && cgo

package platform

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdlib.h>
int gsNotifyInit(void);
void gsNotifyPost(const char *title, const char *body);
*/
import "C"

import (
	"sync"
	"unsafe"
)

// 맥은 앱 묶음(.app) 안에서 돌 때 UserNotifications 로 이 앱 이름을 걸고 알림을 보낸다.
// osascript 로 보내면 '스크립트 편집기'의 알림이 되어, 누르면 스크립트 편집기가 열린다.
// 앱 묶음 밖(go run·테스트)에서는 이 API 를 부르면 죽으므로 osascript 로 돌아간다.

var (
	notifyOnce  sync.Once
	notifyReady bool
	notifyMu    sync.Mutex
	notifyClick func()
	notifyError func(string)
)

func setupNotify(onClick func(), onError func(string)) {
	notifyMu.Lock()
	notifyClick, notifyError = onClick, onError
	notifyMu.Unlock()
	nativeReady()
}

// nativeReady 는 처음 부를 때 알림 받는 쪽을 등록하고 권한을 묻는다 (처음 한 번 맥이 허용 여부를 묻는다).
func nativeReady() bool {
	notifyOnce.Do(func() { notifyReady = C.gsNotifyInit() == 1 })
	return notifyReady
}

func nativeNotify(title, message string) bool {
	if !nativeReady() {
		return false
	}
	ct, cm := C.CString(title), C.CString(message)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cm))
	C.gsNotifyPost(ct, cm)
	return true
}

//export gsNotifyClicked
func gsNotifyClicked() {
	notifyMu.Lock()
	fn := notifyClick
	notifyMu.Unlock()
	if fn != nil {
		go fn()
	}
}

//export gsNotifyFailed
func gsNotifyFailed(msg *C.char) {
	notifyMu.Lock()
	fn := notifyError
	notifyMu.Unlock()
	if fn != nil {
		fn(C.GoString(msg))
	}
}
