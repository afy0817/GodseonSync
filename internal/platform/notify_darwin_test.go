//go:build darwin && cgo

package platform

import "testing"

// 테스트 바이너리는 앱 묶음 밖이라 UserNotifications 를 건드리지 않고(건드리면 죽는다) osascript 로 돌아가야 한다.
func TestNativeNotifyOutsideBundle(t *testing.T) {
	if nativeReady() {
		t.Fatal("앱 묶음 밖인데 기본 알림을 쓰려 함")
	}
}
