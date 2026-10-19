// Package window 는 설정 화면을 OS 기본 웹뷰(맥 WebKit, 윈도우 WebView2)로 띄우는 전용 창이다.
//
// 창은 트레이 에이전트와 다른 프로세스(`GodseonSync window`)로 띄운다. 트레이와 창이 둘 다
// 메인 스레드의 이벤트 루프를 써야 해서 한 프로세스에 두면 서로 부딪힌다. 창을 닫으면 그 프로세스만 끝나고
// 백그라운드 동기화는 계속된다. 리눅스는 기본 브라우저로 연다.
package window

import "github.com/afy0817/GodseonSync/internal/i18n"

// ErrUnsupported 는 이 OS 에서는 전용 창을 쓸 수 없다는 뜻이다.
var ErrUnsupported = i18n.Error("이 OS 에서는 전용 창을 쓸 수 없습니다")

const (
	Title  = "GodseonSync"
	Width  = 1040
	Height = 820
)
