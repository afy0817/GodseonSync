//go:build windows

package window

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/store"
)

var (
	mu      sync.Mutex
	current webview2.WebView

	user32                      = windows.NewLazySystemDLL("user32.dll")
	setDpiAwarenessCtx          = user32.NewProc("SetProcessDpiAwarenessContext")
	getDpiForSystem             = user32.NewProc("GetDpiForSystem")
	showWindow                  = user32.NewProc("ShowWindow")
	setForegroundWindow         = user32.NewProc("SetForegroundWindow")
	perMonitorAwareV2           = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	swRestore           uintptr = 9
)

// Supported 는 전용 창을 쓸 수 있는지 알려 준다 (WebView2 가 없으면 Run 이 실패하고 브라우저로 연다).
func Supported() bool { return true }

// Run 은 창을 띄우고 창이 닫힐 때까지 돌아오지 않는다.
func Run(url string) error {
	// 고해상도 화면에서 흐릿하지 않게 하고, 창 크기는 화면 배율에 맞춘다
	scale := uint(96)
	if setDpiAwarenessCtx.Find() == nil {
		setDpiAwarenessCtx.Call(perMonitorAwareV2)
		if getDpiForSystem.Find() == nil {
			if dpi, _, _ := getDpiForSystem.Call(); dpi > 0 {
				scale = uint(dpi)
			}
		}
	}
	wv := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(store.Home(), "webview2"),
		WindowOptions: webview2.WindowOptions{
			Title:  Title,
			Width:  Width * scale / 96,
			Height: Height * scale / 96,
			IconId: 2, // 실행 파일에 넣은 앱 아이콘 (packaging/windows/winres.json: 매니페스트가 1번, 아이콘 묶음이 2번)
			Center: true,
		},
	})
	if wv == nil {
		return errors.New(i18n.T("WebView2 를 시작할 수 없습니다 (Microsoft Edge WebView2 런타임이 필요합니다)"))
	}
	mu.Lock()
	current = wv
	mu.Unlock()
	// 백그라운드 에이전트가 띄운 프로세스라 그냥 두면 다른 창 뒤에 뜬다. 연 쪽(알림·트레이)이 AllowSetForegroundWindow 로 넘긴 권한으로 앞으로 나온다.
	setForegroundWindow.Call(uintptr(wv.Window()))
	bindPickFolder(wv)
	wv.Navigate(url)
	wv.Run()
	mu.Lock()
	current = nil
	mu.Unlock()
	wv.Destroy()
	return nil
}

// bindPickFolder 는 화면에 gsPickFolder(initial, prompt) 를 만든다. 고른 결과는 gsFolderPicked({path, error}) 로 돌려준다.
// 폴더 선택 창은 이 창 프로세스가 설정 창에 붙여(모달) 띄운다. 트레이 프로세스가 띄우면 윈도우가 앞으로 못 나오게 해서
// 설정 창 뒤에 숨고, 응답 없는 요청이 쌓여 화면의 다른 요청까지 막혔다 (WebView2 는 한 주소에 연결 6개).
func bindPickFolder(wv webview2.WebView) {
	var picking atomic.Bool
	_ = wv.Bind("gsPickFolder", func(initial, prompt string) bool {
		if !picking.CompareAndSwap(false, true) {
			return false
		}
		// WebView2 이벤트 처리 중에 모달 창을 띄우면 다시 불려 들어올 수 있어, 처리를 끝낸 뒤 UI 스레드에서 띄운다
		wv.Dispatch(func() {
			p, err := platform.PickFolderIn(uintptr(wv.Window()), initial, prompt)
			picking.Store(false)
			res := map[string]string{"path": p}
			if err != nil {
				res["error"] = err.Error()
			}
			b, _ := json.Marshal(res)
			// 선택 창이 떠 있는 동안 들어온 Dispatch(창 앞으로·닫기)는 깨우는 스레드 메시지가 모달 루프에서 버려져 큐에 남는다.
			// 결과도 Dispatch 로 보내면 새 메시지가 그것들까지 같이 처리한다.
			wv.Dispatch(func() { wv.Eval("window.gsFolderPicked && window.gsFolderPicked(" + string(b) + ")") })
		})
		return true
	})
}

// Focus 는 창을 맨 앞으로 가져온다.
func Focus() {
	mu.Lock()
	wv := current
	mu.Unlock()
	if wv == nil {
		return
	}
	wv.Dispatch(func() {
		hwnd := uintptr(wv.Window())
		showWindow.Call(hwnd, swRestore)
		setForegroundWindow.Call(hwnd)
	})
}

// Close 는 창을 닫는다.
func Close() {
	mu.Lock()
	wv := current
	mu.Unlock()
	if wv != nil {
		wv.Dispatch(wv.Terminate)
	}
}
