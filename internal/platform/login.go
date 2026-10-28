package platform

import "time"

// loginWindow 는 로그인한 뒤 이 시간 안에 시작하면 로그인 자동 실행으로 보는 시간이다.
// 맥 로그인 항목은 로그인하고 1분 안에 떴다 (2026-10-26~28 다섯 번 모두 같은 분).
const loginWindow = 2 * time.Minute

// JustLoggedIn 은 이 사용자가 로그인한 지 얼마 안 됐는지다 (로그인 시각을 모르면 false).
// OS 가 인자 없이 띄운 실행(맥 '로그인 항목' 등)을 로그인 자동 실행으로 알아보는 데 쓴다.
func JustLoggedIn() bool {
	t := loginTime()
	return !t.IsZero() && time.Since(t) < loginWindow
}
