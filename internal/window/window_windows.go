//go:build windows

package window

import (
	"errors"
	"path/filepath"
	"sync"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/afy0817/GodseonSync/internal/i18n"
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
			IconId: 2, // 실행 파일에 넣은 앱 아이콘 (rsrc: 매니페스트가 1번, 아이콘 묶음이 2번)
			Center: true,
		},
	})
	if wv == nil {
		return errors.New(i18n.T("WebView2 를 시작할 수 없습니다 (Microsoft Edge WebView2 런타임이 필요합니다)"))
	}
	mu.Lock()
	current = wv
	mu.Unlock()
	wv.Navigate(url)
	wv.Run()
	mu.Lock()
	current = nil
	mu.Unlock()
	wv.Destroy()
	return nil
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
