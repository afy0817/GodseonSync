//go:build !windows && (!darwin || !cgo)

package platform

import "time"

// loginTime 은 리눅스(자동 실행 항목에 인자가 있어 필요 없음)와 cgo 없는 맥 빌드에서는 모른다.
func loginTime() time.Time { return time.Time{} }
