//go:build darwin && cgo

package platform

/*
#include <stdlib.h>
#include <string.h>
#include <utmpx.h>

// gsConsoleLogin 은 이 사용자가 화면(console)으로 로그인한 시각(초)이다. 기록이 없으면 0.
static long long gsConsoleLogin(const char *user) {
	long long t = 0;
	struct utmpx *u;
	setutxent();
	while ((u = getutxent()) != NULL) {
		if (u->ut_type == USER_PROCESS && strcmp(u->ut_line, "console") == 0 &&
			strncmp(u->ut_user, user, sizeof(u->ut_user)) == 0 && u->ut_tv.tv_sec > t) {
			t = u->ut_tv.tv_sec;
		}
	}
	endutxent();
	return t;
}
*/
import "C"

import (
	"os/user"
	"time"
	"unsafe"
)

// loginTime 은 이 사용자가 맥에 로그인한 시각이다 (`who` 의 console 줄과 같은 기록). 모르면 0.
func loginTime() time.Time {
	u, err := user.Current()
	if err != nil {
		return time.Time{}
	}
	name := C.CString(u.Username)
	defer C.free(unsafe.Pointer(name))
	sec := int64(C.gsConsoleLogin(name))
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
