// Package agent 는 백그라운드에 상주하는 GodseonSync 다.
// 트레이(맥은 메뉴 막대) 아이콘을 띄우고, 정해진 주기마다 동기화하고, 설정 화면 서버를 연다.
// 한 PC 에서 하나만 돈다. 이미 떠 있으면 새로 띄우지 않고 그쪽 설정 화면을 연다.
package agent

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/afy0817/GodseonSync/internal/engine"
	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/icon"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/server"
	"github.com/afy0817/GodseonSync/internal/store"
	"github.com/afy0817/GodseonSync/internal/window"
)

// Options 는 에이전트 실행 방법이다.
type Options struct {
	OpenUI bool // 시작하자마자 설정 화면을 연다
	NoTray bool // 트레이 아이콘 없이 (서버용 리눅스 등)
}

type uiInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// ExistingURL 은 이미 떠 있는 GodseonSync 의 설정 화면 주소다 (없으면 빈 문자열).
func ExistingURL() string {
	var info uiInfo
	if store.ReadJSON(store.UIFile(), &info) != nil || info.Port == 0 {
		return ""
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/ping", info.Port), nil)
	req.Header.Set("X-GS-Token", info.Token)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d/#t=%s", info.Port, info.Token)
}

// Agent 는 상주 프로세스다.
type Agent struct {
	srv     *server.Server
	trigger chan engine.Options
	wake    chan struct{}
	quit    chan struct{}
	once    sync.Once

	mu            sync.Mutex
	running       bool
	tray          *trayMenu
	noWindow      bool      // 전용 창을 못 띄워서 브라우저로 연다
	windowStarted time.Time // 창 프로세스를 띄운 시각 (두 번 띄우지 않게)
}

// RequestOpenUI 는 이미 떠 있는 GodseonSync 에 설정 창을 열라고 부탁한다.
func RequestOpenUI() error {
	var info uiInfo
	if err := store.ReadJSON(store.UIFile(), &info); err != nil {
		return err
	}
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/ui/open", info.Port), strings.NewReader("{}"))
	req.Header.Set("X-GS-Token", info.Token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Run 은 에이전트를 실행한다 (끝날 때까지 돌아오지 않음).
func Run(opt Options) error {
	if url := ExistingURL(); url != "" {
		if opt.OpenUI {
			if err := RequestOpenUI(); err != nil {
				return platform.OpenBrowser(url)
			}
		}
		return nil // 이미 떠 있다
	}
	a := &Agent{trigger: make(chan engine.Options, 4), wake: make(chan struct{}, 1), quit: make(chan struct{})}
	a.srv = server.New(a)
	l, err := a.srv.Listen()
	if err != nil {
		return fmt.Errorf(i18n.T("설정 화면 서버를 열 수 없습니다: %w"), err)
	}
	if err := store.WriteJSON(store.UIFile(), uiInfo{Port: a.srv.Port, Token: a.srv.Token, PID: os.Getpid()}); err != nil {
		return err
	}
	defer func() {
		var info uiInfo
		if store.ReadJSON(store.UIFile(), &info) == nil && info.PID == os.Getpid() {
			_ = os.Remove(store.UIFile())
		}
	}()
	store.Log("GodseonSync 시작 (설정 화면 포트 %d)", a.srv.Port)
	platform.SetupNotifications(a.OpenUI, func(msg string) {
		store.Log("OS 알림을 보내지 못했습니다 (시스템 설정 › 알림에서 GodseonSync 를 허용하세요): %s", msg)
	})
	go func() { _ = a.srv.Serve(l) }()
	go a.scheduler()
	if opt.OpenUI { // 로그인 자동 실행(--background)이면 처음 설정 전이라도 트레이에만 알린다
		a.OpenUI()
	}
	if opt.NoTray {
		<-a.quit
		return nil
	}
	systray.Run(a.onTrayReady, func() {})
	return nil
}

// ------------------------------------------------------------ server.Controller

// StartSync 는 지금 동기화를 시작한다.
func (a *Agent) StartSync(opt engine.Options) error {
	if store.ReadLock() != nil {
		return errors.New(i18n.T("이미 동기화 중입니다"))
	}
	if !store.LoadSettings().Configured {
		return engine.ErrNotConfigured
	}
	select {
	case a.trigger <- opt:
		return nil
	default:
		return errors.New(i18n.T("동기화 요청이 밀려 있습니다. 잠시 뒤 다시 누르세요"))
	}
}

// SettingsChanged 는 주기·일시 정지 같은 설정이 바뀌었다고 알린다.
func (a *Agent) SettingsChanged() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
	a.refreshTray()
}

// OpenUI 는 설정 창을 연다. 이미 열려 있으면 맨 앞으로 가져온다.
// 전용 창을 쓸 수 없는 OS 이거나 창을 띄우지 못하면 기본 브라우저로 연다.
func (a *Agent) OpenUI() {
	url := a.srv.URL()
	if a.srv.WindowActive() {
		a.srv.SendWindow("focus")
		return
	}
	a.mu.Lock()
	if a.noWindow || !window.Supported() {
		a.mu.Unlock()
		_ = platform.OpenBrowser(url)
		return
	}
	if time.Since(a.windowStarted) < 10*time.Second { // 방금 띄우는 중
		a.mu.Unlock()
		return
	}
	a.windowStarted = time.Now()
	a.mu.Unlock()

	cmd := exec.Command(platform.Executable(), "window")
	if f, err := os.OpenFile(filepath.Join(store.Home(), "window.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close()
	}
	if err := cmd.Start(); err != nil {
		store.Log("설정 창을 띄우지 못해 브라우저로 엽니다: %v", err)
		_ = platform.OpenBrowser(url)
		return
	}
	started := time.Now()
	go func() {
		err := cmd.Wait()
		a.mu.Lock()
		a.windowStarted = time.Time{}
		a.mu.Unlock()
		if err != nil && time.Since(started) < 20*time.Second {
			// WebView2 가 없는 윈도우 등: 다음부터는 브라우저로 연다
			store.Log("설정 창이 바로 닫혀서 브라우저로 엽니다: %v", err)
			a.mu.Lock()
			a.noWindow = true
			a.mu.Unlock()
			_ = platform.OpenBrowser(url)
		}
	}()
}

// Quit 은 GodseonSync 를 끝낸다.
func (a *Agent) Quit() {
	a.once.Do(func() {
		a.srv.SendWindow("close")
		// 동기화 중이면 rclone 을 안전하게 멈추고, 기록을 남기고 끝날 때까지 기다린다 (최대 90초)
		engine.Interrupt()
		for deadline := time.Now().Add(90 * time.Second); a.isRunning() && time.Now().Before(deadline); {
			time.Sleep(200 * time.Millisecond)
		}
		store.Log("GodseonSync 종료")
		time.Sleep(300 * time.Millisecond)
		close(a.quit)
		systray.Quit()
	})
}

// ------------------------------------------------------------ 예약 동기화

func (a *Agent) scheduler() {
	next := time.Now().Add(15 * time.Second) // 켜고 조금 뒤 첫 동기화
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		var opt *engine.Options
		select {
		case <-a.quit:
			return
		case o := <-a.trigger:
			opt = &o
		case <-a.wake:
			s := store.LoadSettings()
			if candidate := time.Now().Add(time.Duration(s.IntervalMinutes) * time.Minute); candidate.Before(next) {
				next = candidate
			}
			continue
		case <-ticker.C:
			s := store.LoadSettings()
			if time.Now().Before(next) || !s.Configured || s.Paused {
				continue
			}
			opt = &engine.Options{}
		}
		select {
		case <-a.quit:
			return
		default:
		}
		a.runSync(*opt)
		next = time.Now().Add(time.Duration(store.LoadSettings().IntervalMinutes) * time.Minute)
	}
}

func (a *Agent) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *Agent) runSync(opt engine.Options) {
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()
	a.refreshTray()
	stop := make(chan struct{})
	go a.animate(stop)
	res, err := engine.Run(opt)
	close(stop)
	a.mu.Lock()
	a.running = false
	a.mu.Unlock()
	a.refreshTray()
	if err != nil {
		if !errors.Is(err, engine.ErrNotConfigured) {
			platform.Notify("GodseonSync", i18n.T("동기화 중단: %v", err))
		}
	} else if len(res.Failed) > 0 {
		platform.Notify("GodseonSync", i18n.T("동기화 실패: %s", strings.Join(res.Failed, ", ")))
	}
	if err == nil && len(res.NeedChoice) > 0 {
		platform.Notify("GodseonSync", i18n.T("이 PC 와 드라이브에 둘 다 파일이 있어 멈춘 폴더가 있습니다. 설정 화면에서 어느 쪽으로 맞출지 고르세요: %s", strings.Join(res.NeedChoice, ", ")))
	}
}

// ------------------------------------------------------------ 트레이

type trayMenu struct {
	status, open, sync, pause, quit *systray.MenuItem
}

var (
	iconCache   = map[string][]byte{}
	iconCacheMu sync.Mutex
)

// trayIcon 은 상태별 아이콘을 만든다 (맥은 흑백 템플릿, 윈도우는 ICO, 리눅스는 PNG).
func trayIcon(state icon.State, rotate float64) (template, regular []byte) {
	key := fmt.Sprintf("%d-%v", state, rotate)
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()
	if b, ok := iconCache[key+"t"]; ok {
		return b, iconCache[key+"r"]
	}
	template = icon.PNG(icon.Render(44, icon.TemplateStyle(state, rotate)))
	if platform.OSKey == "windows" {
		regular = icon.ICO(icon.TrayImages(state, rotate))
	} else {
		regular = icon.PNG(icon.Render(64, icon.TrayStyle(state, rotate)))
	}
	iconCache[key+"t"], iconCache[key+"r"] = template, regular
	return template, regular
}

func setTrayIcon(state icon.State, rotate float64) {
	t, r := trayIcon(state, rotate)
	if platform.OSKey == "mac" {
		systray.SetTemplateIcon(t, r)
	} else {
		systray.SetIcon(r)
	}
}

func (a *Agent) onTrayReady() {
	setTrayIcon(icon.Normal, 0)
	systray.SetTooltip("GodseonSync")
	m := &trayMenu{}
	m.status = systray.AddMenuItem("", "")
	m.status.Disable()
	systray.AddSeparator()
	m.open = systray.AddMenuItem("", "")
	m.sync = systray.AddMenuItem("", "")
	m.pause = systray.AddMenuItemCheckbox("", "", false)
	systray.AddSeparator()
	m.quit = systray.AddMenuItem("", "") // 이름은 refreshTray 가 이 PC 언어로 쓴다
	a.mu.Lock()
	a.tray = m
	a.mu.Unlock()
	systray.SetOnTapped(a.OpenUI) // 윈도우: 아이콘 클릭으로 바로 열기
	a.refreshTray()
	go func() {
		t := time.NewTicker(30 * time.Second) // "3분 전" 같은 표시를 갱신
		defer t.Stop()
		for {
			select {
			case <-a.quit:
				return
			case <-m.open.ClickedCh:
				a.OpenUI()
			case <-m.sync.ClickedCh:
				if err := a.StartSync(engine.Options{}); err != nil {
					platform.Notify("GodseonSync", err.Error())
				}
			case <-m.pause.ClickedCh:
				s := store.LoadSettings()
				s.Paused = !s.Paused
				_ = store.SaveSettings(s)
				a.SettingsChanged()
			case <-m.quit.ClickedCh:
				a.Quit()
			case <-t.C:
				a.refreshTray()
			}
		}
	}()
}

func (a *Agent) refreshTray() {
	a.mu.Lock()
	m, running := a.tray, a.running
	a.mu.Unlock()
	if m == nil {
		return
	}
	s := store.LoadSettings() // 언어도 맞춘다 (설정 화면에서 바꾸면 SettingsChanged → 여기)
	st := store.LoadStatus()
	text, state := statusLine(s, st, running)
	m.status.SetTitle(text)
	m.open.SetTitle(i18n.T("설정 열기…"))
	m.open.SetTooltip(i18n.T("GodseonSync 설정 화면을 엽니다"))
	m.sync.SetTitle(i18n.T("지금 동기화"))
	m.sync.SetTooltip(i18n.T("모든 폴더를 지금 동기화합니다"))
	m.pause.SetTitle(i18n.T("자동 동기화 일시 정지"))
	m.quit.SetTitle(i18n.T("GodseonSync 종료"))
	m.quit.SetTooltip(i18n.T("백그라운드 동기화를 멈추고 끝냅니다"))
	systray.SetTooltip("GodseonSync — " + text)
	if s.Paused {
		m.pause.Check()
	} else {
		m.pause.Uncheck()
	}
	if !running {
		setTrayIcon(state, 0)
	}
}

func statusLine(s store.Settings, st *store.Status, running bool) (string, icon.State) {
	switch {
	case !s.Configured:
		return i18n.T("처음 설정이 필요합니다"), icon.Error
	case running:
		if lock := store.ReadLock(); lock != nil && lock.Folder != "" {
			return i18n.T("동기화 중: %s", lock.Folder), icon.Normal
		}
		return i18n.T("동기화 중…"), icon.Normal
	case st.LastError != "":
		return i18n.T("오류: %s", short(st.LastError)), icon.Error
	}
	for _, f := range st.Folders {
		if f.LastError != "" {
			return i18n.T("일부 폴더 오류 — 설정 화면에서 확인"), icon.Error
		}
	}
	for _, f := range st.Folders {
		if f.NeedsChoice {
			return i18n.T("결정 필요 — 이 PC 와 드라이브 중 어느 쪽으로 맞출지 고르세요"), icon.Error
		}
	}
	prefix := ""
	if s.Paused {
		prefix = i18n.T("일시 정지 · ")
	}
	state := icon.Normal
	if s.Paused {
		state = icon.Paused
	}
	if st.LastRun == "" {
		return prefix + i18n.T("아직 동기화 전"), state
	}
	return prefix + i18n.T("마지막 동기화 %s", ago(st.LastRun)), state
}

func (a *Agent) animate(stop chan struct{}) {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	deg := 0.0
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			a.mu.Lock()
			ready := a.tray != nil
			a.mu.Unlock()
			if ready {
				deg = float64((int(deg) + 45) % 360)
				setTrayIcon(icon.Normal, deg)
			}
		}
	}
}

func short(s string) string {
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

func ago(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return i18n.T("방금")
	case d < time.Hour:
		return i18n.T("%d분 전", int(d.Minutes()))
	case d < 24*time.Hour:
		return i18n.T("%d시간 전", int(d.Hours()))
	default:
		return t.Format(i18n.T("1월 2일 15:04"))
	}
}
