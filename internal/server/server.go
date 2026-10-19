// Package server 는 설정 화면용 작은 웹 서버다. 127.0.0.1 에서만 열리고, 화면은 기본 브라우저로 띄운다.
//
// 다른 웹사이트가 이 서버를 부르지 못하게 API 는 실행할 때마다 새로 만드는 토큰(X-GS-Token 헤더)을 요구하고,
// Host 헤더가 127.0.0.1/localhost 가 아니면 거절한다.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/afy0817/GodseonSync/internal/buildinfo"
	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/engine"
	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/rclone"
	"github.com/afy0817/GodseonSync/internal/storage"
	"github.com/afy0817/GodseonSync/internal/store"
	"github.com/afy0817/GodseonSync/web"
)

// PreferredPort 를 먼저 쓰고, 사용 중이면 아무 포트나 쓴다. 9417 은 IANA 에 등록되지 않았고
// 맥·윈도우(49152~)·리눅스(32768~) 의 임시 포트 범위 밖이라 다른 프로그램과 잘 겹치지 않는다.
const PreferredPort = 9417

// Controller 는 서버가 에이전트(트레이·예약 동기화)에 시키는 일이다.
type Controller interface {
	StartSync(opt engine.Options) error
	SettingsChanged()
	OpenUI()
	Quit()
}

// Server 는 설정 화면 서버다.
type Server struct {
	Token string
	Port  int
	ctrl  Controller
	mu    sync.Mutex
	job   *rclone.ConnectJob
	// migrating 은 저장소를 옮기는 중인지다 (migrate.go). 그동안 드라이브 설정 받기·저장을 막는다.
	migrating atomic.Bool

	// 전용 설정 창 프로세스와의 연락 (창이 롱 폴링으로 명령을 기다린다)
	winMu      sync.Mutex
	winWaiters map[chan string]bool
	winPending string
	winLast    time.Time
}

// New 는 서버를 만든다.
func New(ctrl Controller) *Server {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return &Server{Token: base64.RawURLEncoding.EncodeToString(b), ctrl: ctrl, winWaiters: map[chan string]bool{}}
}

// WindowActive 는 전용 설정 창이 열려 있는지 알려 준다.
func (s *Server) WindowActive() bool {
	s.winMu.Lock()
	defer s.winMu.Unlock()
	return len(s.winWaiters) > 0 || time.Since(s.winLast) < 3*time.Second
}

// SendWindow 는 열려 있는 설정 창에 명령(focus / close)을 보낸다.
func (s *Server) SendWindow(cmd string) {
	s.winMu.Lock()
	defer s.winMu.Unlock()
	if len(s.winWaiters) == 0 {
		s.winPending = cmd // 다음 폴링 때 전한다
		return
	}
	for ch := range s.winWaiters {
		select {
		case ch <- cmd:
		default:
		}
	}
}

func (s *Server) windowPoll(r *http.Request) (any, error) {
	ch := make(chan string, 1)
	s.winMu.Lock()
	if s.winPending != "" {
		ch <- s.winPending
		s.winPending = ""
	}
	s.winWaiters[ch] = true
	s.winMu.Unlock()
	defer func() {
		s.winMu.Lock()
		delete(s.winWaiters, ch)
		s.winLast = time.Now()
		s.winMu.Unlock()
	}()
	select {
	case cmd := <-ch:
		return map[string]string{"cmd": cmd}, nil
	case <-time.After(25 * time.Second):
		return map[string]string{"cmd": "none"}, nil
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
}

func (s *Server) openUI(r *http.Request) (any, error) {
	s.ctrl.OpenUI()
	return map[string]bool{"ok": true}, nil
}

// connectOpen 은 브라우저가 자동으로 안 열렸을 때 구글 로그인 주소를 기본 브라우저로 연다.
func (s *Server) connectOpen(r *http.Request) (any, error) {
	s.mu.Lock()
	job := s.job
	s.mu.Unlock()
	if job == nil || job.Snapshot()["url"] == "" {
		return nil, fail(http.StatusBadRequest, "로그인 주소가 아직 없습니다.")
	}
	return map[string]bool{"ok": true}, platform.OpenBrowser(job.Snapshot()["url"])
}

// Listen 은 127.0.0.1 의 포트를 연다.
func (s *Server) Listen() (net.Listener, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", PreferredPort))
	if err != nil {
		l, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return nil, err
	}
	s.Port = l.Addr().(*net.TCPAddr).Port
	return l, nil
}

// URL 은 브라우저로 열 주소다 (토큰은 주소의 # 뒤에 있어 서버 로그·Referer 로 새지 않는다).
func (s *Server) URL() string { return fmt.Sprintf("http://127.0.0.1:%d/#t=%s", s.Port, s.Token) }

// Serve 는 요청을 처리한다 (돌아오지 않음).
func (s *Server) Serve(l net.Listener) error {
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(l)
}

type apiError struct {
	status int
	msg    string
	extra  map[string]any
}

func (e *apiError) Error() string { return e.msg }

// fail 은 화면에 보낼 오류다. 문구는 이 PC 언어로 바꾼다 (i18n.T — 한국어 문구가 키).
func fail(status int, format string, args ...any) *apiError {
	return &apiError{status: status, msg: i18n.T(format, args...)}
}

type handler func(s *Server, r *http.Request) (any, error)

var routes = map[string]handler{
	"GET /api/ping":            (*Server).ping,
	"GET /api/overview":        (*Server).overview,
	"GET /api/status":          (*Server).status,
	"GET /api/config":          (*Server).getConfig,
	"GET /api/drive/ls":        (*Server).driveLs,
	"GET /api/log":             (*Server).log,
	"POST /api/connect":        (*Server).connect,
	"POST /api/connect/cancel": (*Server).connectCancel,
	"POST /api/connect/open":   (*Server).connectOpen,
	"GET /api/window/poll":     (*Server).windowPoll,
	"POST /api/ui/open":        (*Server).openUI,
	"POST /api/disconnect":     (*Server).disconnect,
	"POST /api/setup/probe":    (*Server).probe,
	"POST /api/setup/finish":   (*Server).finishSetup,
	"POST /api/reset":          (*Server).reset,
	"POST /api/config/save":    (*Server).saveConfig,
	"POST /api/client/rename":  (*Server).renameClient,
	"POST /api/client/remove":  (*Server).removeClient,
	"POST /api/sync":           (*Server).sync,
	"POST /api/folder/choose":  (*Server).chooseFolder,
	"GET /api/notices":         (*Server).notices,
	"POST /api/notices":        (*Server).addNotice,
	"POST /api/notices/read":   (*Server).readNotices,
	"POST /api/notices/delete": (*Server).deleteNotices,
	"POST /api/background":     (*Server).background,
	"POST /api/pick-folder":    (*Server).pickFolder,
	"POST /api/quit":           (*Server).quit,
	"POST /api/log/clear":      (*Server).clearLog,
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	if host != fmt.Sprintf("127.0.0.1:%d", s.Port) && host != fmt.Sprintf("localhost:%d", s.Port) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		sub, _ := fs.Sub(web.FS, ".")
		http.FileServer(http.FS(sub)).ServeHTTP(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-GS-Token")), []byte(s.Token)) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": i18n.T("토큰이 맞지 않습니다. GodseonSync 를 다시 여세요.")})
		return
	}
	h, ok := routes[r.Method+" "+r.URL.Path]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": i18n.T("없는 API")})
		return
	}
	out, err := h(s, r)
	if err != nil {
		var ae *apiError
		if !errors.As(err, &ae) {
			ae = &apiError{status: http.StatusBadRequest, msg: err.Error()}
		}
		body := map[string]any{"error": ae.msg}
		for k, v := range ae.extra {
			body[k] = v
		}
		writeJSON(w, ae.status, body)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	if err := dec.Decode(v); err != nil && err.Error() != "EOF" {
		return fail(http.StatusBadRequest, "요청 형식이 올바르지 않습니다: %v", err)
	}
	return nil
}

// ------------------------------------------------------------ 조회

func (s *Server) ping(r *http.Request) (any, error) { return map[string]bool{"ok": true}, nil }

func (s *Server) connectState() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return nil
	}
	return s.job.Snapshot()
}

func (s *Server) overview(r *http.Request) (any, error) {
	st := store.LoadSettings()
	home, _ := os.UserHomeDir()
	return map[string]any{
		"app_version":   buildinfo.Version,
		"os":            platform.OSKey,
		"hostname":      platform.Hostname(),
		"home":          home,
		"settings":      st,
		"connected":     rclone.New().HasRemote(st.Remote),
		"connection":    connection(st),
		"saved":         savedConnections(),
		"modes":         modeList(),
		"connect":       s.connectState(),
		"autostart":     map[string]any{"enabled": platform.AutostartEnabled(), "problem": platform.AutostartProblem()},
		"data_dir":      store.Home(),
		"custom_client": buildinfo.GoogleClientID != "",
	}, nil
}

// connection 은 이 PC 의 저장소 연결이다: 모드와 화면에 보여도 되는 값(구글이면 권한 범위·클라이언트 ID).
// 연결이 없거나 모르는 종류면 nil.
func connection(st store.Settings) any {
	rc := rclone.New()
	m := storage.Of(rc, st.Remote)
	if m == nil {
		return nil
	}
	conf, _ := rc.RemoteOptions(st.Remote)
	return map[string]any{"mode": m.Key, "label": i18n.T(m.Label), "info": m.Info(conf), "where": m.Where(conf)}
}

// savedName 은 모드를 바꿀 때 그 모드의 연결을 저장해 두는 rclone 연결 이름이다 (gs-saved-gdrive, gs-saved-sftp).
// 다시 그 모드로 돌아오면 로그인 없이 이 연결을 쓴다. 저장소(드라이브·서버)의 내용은 건드리지 않는다.
func savedName(key string) string { return "gs-saved-" + key }

// savedConnections 는 저장해 둔 모드별 연결이다 (모드 → 이름·위치·보여도 되는 값).
func savedConnections() map[string]any {
	rc := rclone.New()
	out := map[string]any{}
	for _, m := range storage.All() {
		if conf, ok := rc.RemoteOptions(savedName(m.Key)); ok && conf["type"] == m.RcloneType {
			out[m.Key] = map[string]any{"label": i18n.T(m.Label), "where": m.Where(conf), "info": m.Info(conf)}
		}
	}
	return out
}

func modeList() []map[string]string {
	out := []map[string]string{}
	for _, m := range storage.All() {
		out = append(out, map[string]string{"key": m.Key, "label": i18n.T(m.Label)})
	}
	return out
}

func (s *Server) status(r *http.Request) (any, error) {
	return map[string]any{"running": store.ReadLock(), "status": store.LoadStatus(), "connect": s.connectState(),
		"unread": store.UnreadNotices()}, nil
}

// ------------------------------------------------------------ 알림 센터

func (s *Server) notices(r *http.Request) (any, error) {
	list := store.LoadNotices()
	unread := 0
	for _, n := range list {
		if !n.Read {
			unread++
		}
	}
	return map[string]any{"items": list, "unread": unread}, nil
}

// addNotice 는 화면에서 생긴 오류를 알림으로 남긴다.
func (s *Server) addNotice(r *http.Request) (any, error) {
	var body struct {
		Level string `json:"level"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if len(body.Title) > 500 {
		body.Title = body.Title[:500]
	}
	if len(body.Body) > 2000 {
		body.Body = body.Body[:2000]
	}
	store.AddNotice(body.Level, body.Title, body.Body)
	return s.notices(r)
}

// readNotices 는 읽음(read 생략 또는 true)·안 읽음 표시를 바꾼다. body: {ids, all, read}
func (s *Server) readNotices(r *http.Request) (any, error) {
	var body struct {
		IDs  []string `json:"ids"`
		All  bool     `json:"all"`
		Read *bool    `json:"read"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if err := store.MarkNotices(body.IDs, body.All, body.Read == nil || *body.Read); err != nil {
		return nil, err
	}
	return s.notices(r)
}

func (s *Server) deleteNotices(r *http.Request) (any, error) {
	var body struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if err := store.DeleteNotices(body.IDs, body.All); err != nil {
		return nil, err
	}
	return s.notices(r)
}

// getConfig 는 공용 설정과 PC 별 상태를 돌려준다.
//   - ?cached=1: 드라이브에 가지 않고 이 PC 가 마지막으로 받아 둔 것을 바로 돌려준다 (화면을 빨리 열기 위해).
//   - 그 밖: 드라이브에서 새로 받는다 (수 초 걸릴 수 있다). 받으면 캐시를 갱신한다.
//     드라이브에 연결할 수 없으면 offline 과 함께 캐시를 돌려준다.
func (s *Server) getConfig(r *http.Request) (any, error) {
	st := store.LoadSettings()
	if !st.Configured {
		return nil, fail(http.StatusBadRequest, "아직 설정되지 않았습니다.")
	}
	if r.URL.Query().Get("cached") == "1" {
		return cachedConfig(st, map[string]any{"cached": true, "offline": false}), nil
	}
	if s.migrating.Load() { // 옮기는 사이 setting.json 이 잠깐 없을 때 Ensure 가 새로 만들면 안 된다
		return cachedConfig(st, map[string]any{"cached": true, "offline": true, "error": i18n.T("저장소를 옮기는 중입니다")}), nil
	}
	d := engine.NewDrive(st)
	// setting.json 과 .status 는 서로 상관없으니 동시에 받는다. Register 는 setting.json 을 받은 뒤에만.
	type statusResult struct {
		m   map[string]config.Status
		err error
	}
	stCh := make(chan statusResult, 1)
	go func() {
		m, err := d.LoadStatuses()
		stCh <- statusResult{m, err}
	}()
	cfg, _, err := d.Ensure(st.ClientName, engine.CachedConfig()) // 없으면 만들고, 예전 형식이면 옮긴다
	if err == nil {
		cfg, err = engine.Register(d, cfg, st) // 다른 PC 가 목록에서 뺐어도 다시 올린다
	}
	sr := <-stCh
	if err == nil {
		err = sr.err
	}
	if err != nil {
		return cachedConfig(st, map[string]any{"cached": true, "offline": true, "error": err.Error()}), nil
	}
	_ = store.WriteJSON(store.CacheFile(), cfg)
	_ = store.WriteJSON(store.StatusesCacheFile(), sr.m)
	return map[string]any{"cached": false, "offline": false, "config": cfg, "statuses": sr.m, "this_client": st.ClientID,
		"synced_at": store.Now(), "warnings": warnings(cfg, st)}, nil
}

// cachedConfig 는 이 PC 에 저장해 둔 마지막 설정·PC 상태로 응답을 만든다 (드라이브에 가지 않는다).
func cachedConfig(st store.Settings, resp map[string]any) map[string]any {
	resp["this_client"] = st.ClientID
	resp["config"] = nil
	resp["statuses"] = map[string]config.Status{}
	var cfg config.Config
	if store.ReadJSON(store.CacheFile(), &cfg) == nil && cfg.Normalize() == nil {
		resp["config"] = &cfg
		resp["warnings"] = warnings(&cfg, st)
		if fi, err := os.Stat(store.CacheFile()); err == nil {
			resp["cached_at"] = fi.ModTime().Format(time.RFC3339)
		}
	}
	statuses := map[string]config.Status{}
	if store.ReadJSON(store.StatusesCacheFile(), &statuses) == nil {
		resp["statuses"] = statuses
	}
	return resp
}

func warnings(cfg *config.Config, st store.Settings) []string {
	out := []string{}
	if errs := config.Validate(cfg); len(errs) > 0 {
		out = append(out, errs...)
	}
	for _, f := range cfg.Folders {
		raw := f.Paths[st.ClientID]
		if raw == "" {
			continue
		}
		p := platform.ExpandPath(raw)
		if _, err := os.Stat(p); err != nil {
			parent := p[:max(0, strings.LastIndexAny(p, `/\`))]
			if fi, err := os.Stat(parent); err != nil || !fi.IsDir() {
				out = append(out, i18n.T("[%s] 이 PC 경로의 상위 폴더가 없습니다: %s", f.Name, parent))
			}
		}
		// 로컬 경로는 길면 짧은 링크로 바꿔 쓰지만 드라이브 경로는 줄일 수 없다
		link := store.Home() + "/links/f-000000000000"
		if config.SessionNameBytes(link, config.Spec(st.Remote, config.BaseOf(rclone.New(), st.Remote), f.DrivePath)) > 255 {
			out = append(out, i18n.T("[%s] 드라이브 경로가 너무 깁니다. 드라이브 경로를 줄이세요.", f.Name))
		}
	}
	return out
}

func (s *Server) driveLs(r *http.Request) (any, error) {
	st := store.LoadSettings()
	p := config.NormDrivePath(r.URL.Query().Get("path"))
	dirs, found, err := rclone.New().LsDirs(engine.NewDrive(st).Spec(p))
	if err != nil {
		return nil, fail(http.StatusBadGateway, "드라이브 목록을 읽지 못했습니다: %v", err)
	}
	out := []string{}
	for _, d := range dirs {
		if !(strings.EqualFold(p, config.RootFolder) && d == config.StatusDir) { // GodseonSync 가 쓰는 폴더는 숨긴다
			out = append(out, d)
		}
	}
	return map[string]any{"path": p, "exists": found, "dirs": out}, nil
}

func (s *Server) log(r *http.Request) (any, error) {
	n, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if n < 10 || n > 3000 {
		n = 300
	}
	return map[string]any{"text": store.LogTail(n), "path": store.LogFile()}, nil
}

func (s *Server) clearLog(r *http.Request) (any, error) {
	if err := store.ClearLog(i18n.T("설정 화면")); err != nil {
		return nil, err
	}
	return map[string]any{"text": store.LogTail(300)}, nil
}

// ------------------------------------------------------------ 처음 설정

// connect 는 저장소 연결(구글이면 브라우저 로그인)을 시작한다. body: {mode, options}.
//
// 이미 연결돼 있으면(클라이언트 ID 를 바꾸는 등 다시 연결) 임시 이름으로 새로 연결하고, 확인이 끝난 뒤에만
// 원래 연결과 바꾼다. 그래서 로그인을 취소하거나 잘못된 클라이언트 ID 로 실패해도 지금 연결로 계속 동기화한다.
func (s *Server) connect(r *http.Request) (any, error) {
	var body struct {
		Mode    string            `json:"mode"`
		Options map[string]string `json:"options"`
		Migrate bool              `json:"migrate"` // 구글: 드라이브 전체 → 이 앱이 만든 파일만 (migrate.go)
		// Move 는 다른 저장소로 바꾸기다 (설정 › 모드에서 다른 모드를 고른 때, switchStorage). 지금 연결은 저장해 두고
		// 지금 저장소는 건드리지 않는다. 새 저장소에 설정이 없으면 지금 설정을 복사해 만든다.
		Move bool `json:"move"`
		// Restore 는 저장해 둔 그 모드의 연결로 바꾸기다 (다시 로그인하지 않는다).
		Restore bool `json:"restore"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	m := storage.Default
	if body.Mode != "" {
		if m = storage.Get(body.Mode); m == nil {
			return nil, fail(http.StatusBadRequest, "모르는 저장소 방식입니다: %s", body.Mode)
		}
	}
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	st := store.LoadSettings()
	rc := rclone.New()
	if body.Restore {
		return s.restoreConnection(rc, st, m)
	}
	if body.Migrate {
		conf, ok := rc.RemoteOptions(st.Remote)
		switch {
		case m != storage.GoogleDrive || storage.Of(rc, st.Remote) != storage.GoogleDrive:
			return nil, fail(http.StatusBadRequest, "권한 범위 옮기기는 구글 드라이브 연결에서만 할 수 있습니다.")
		case !st.Configured || !ok:
			return nil, fail(http.StatusBadRequest, "처음 설정을 마친 뒤에 할 수 있습니다.")
		case conf["scope"] == "drive.file":
			return nil, fail(http.StatusBadRequest, "이미 '이 앱이 만든 파일만' 범위입니다.")
		}
		body.Options = storage.FileScopeOptions(body.Options, conf)
	}
	params, err := m.Params(body.Options)
	if err != nil {
		return nil, err
	}
	var own map[string]string // GodseonSync 전용 값 (gs_path 등): rclone 에 넘기지 않고 만든 뒤 rclone.conf 에 쓴다
	rcParams := params[:0:0]
	for _, p := range params {
		if k, v, ok := strings.Cut(p, "="); ok && strings.HasPrefix(k, "gs_") {
			if own == nil {
				own = map[string]string{}
			}
			own[k] = v
			continue
		}
		rcParams = append(rcParams, p)
	}
	params = append(rcParams, m.CreateFlags...)
	if body.Move && (!st.Configured || !rc.HasRemote(st.Remote)) {
		body.Move = false // 처음 설정이면 옮길 것이 없다
	}
	s.mu.Lock()
	if s.job != nil {
		if s.job.Snapshot()["finishing"] == "1" {
			s.mu.Unlock()
			return nil, fail(http.StatusConflict, "앞서 연결한 것을 확인하는 중입니다. 끝난 뒤 다시 하세요.")
		}
		s.job.Cancel()
	}
	s.mu.Unlock()
	name, hooks := st.Remote, rclone.ConnectHooks{}
	if rc.HasRemote(st.Remote) {
		name = st.Remote + pendingSuffix
		if rc.HasRemote(name) { // 지난번에 끝내지 못하고 남은 것
			if err := rc.DeleteRemote(name); err != nil {
				return nil, err
			}
		}
		finish, failTitle := func(progress func(string)) error { return switchRemote(rc, st, name, progress) }, i18n.T("다시 연결하지 못했습니다")
		switch {
		case body.Migrate:
			finish, failTitle = func(progress func(string)) error { return s.migrateScope(rc, st, name, progress) }, i18n.T("저장소를 옮기지 못했습니다")
		case body.Move:
			finish, failTitle = func(progress func(string)) error { return s.switchStorage(rc, st, name, m, progress) }, i18n.T("저장소를 바꾸지 못했습니다")
		}
		if m.Verify {
			inner := finish
			finish = func(progress func(string)) error {
				if err := verifyRemote(rc, name, progress); err != nil {
					return err
				}
				return inner(progress)
			}
		}
		hooks.Finish = func(progress func(string)) error {
			err := finish(progress)
			if err != nil {
				store.AddNotice("err", failTitle, err.Error())
			}
			return err
		}
		hooks.Cleanup = func() { _ = rc.DeleteRemote(name) }
	} else if m.Verify {
		// 처음 연결: 만든 뒤 실제로 접속되는지 확인하고, 안 되면 지운다 (잘못된 주소·비밀번호로 '연결됨' 이 되지 않게)
		hooks.Finish = func(progress func(string)) error {
			err := verifyRemote(rc, name, progress)
			if err != nil {
				store.AddNotice("err", i18n.T("연결하지 못했습니다"), err.Error())
			}
			return err
		}
		hooks.Cleanup = func() { _ = rc.DeleteRemote(name) }
	}
	if own != nil {
		inner := hooks.Finish
		hooks.Finish = func(progress func(string)) error {
			if err := rc.SetOptions(name, own); err != nil {
				return err
			}
			if inner != nil {
				return inner(progress)
			}
			return nil
		}
	}
	job, err := rc.StartConnect(name, m.RcloneType, params, hooks)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.job = job
	s.mu.Unlock()
	return map[string]any{"connect": job.Snapshot()}, nil
}

// restoreConnection 은 저장해 둔 m 모드의 연결로 바꾸는 작업을 시작한다 (로그인 없음, switchStorage 와 같은 확인).
func (s *Server) restoreConnection(rc *rclone.Client, st store.Settings, m *storage.Mode) (any, error) {
	saved := savedName(m.Key)
	if conf, ok := rc.RemoteOptions(saved); !ok || conf["type"] != m.RcloneType {
		return nil, fail(http.StatusBadRequest, "저장해 둔 %s 연결이 없습니다. 새로 연결하세요.", i18n.T(m.Label))
	}
	if !st.Configured || !rc.HasRemote(st.Remote) {
		return nil, fail(http.StatusBadRequest, "처음 설정을 마친 뒤에 할 수 있습니다.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil {
		if s.job.Snapshot()["finishing"] == "1" && s.job.Snapshot()["state"] == "running" {
			return nil, fail(http.StatusConflict, "앞서 연결한 것을 확인하는 중입니다. 끝난 뒤 다시 하세요.")
		}
		s.job.Cancel()
	}
	s.job = rclone.StartTask(saved, func(progress func(string)) error {
		err := s.switchStorage(rc, st, saved, m, progress)
		if err != nil {
			store.AddNotice("err", i18n.T("저장소를 바꾸지 못했습니다"), err.Error())
		}
		return err
	})
	return map[string]any{"connect": s.job.Snapshot()}, nil
}

// verifyRemote 는 새로 만든 연결로 저장소 맨 위를 읽어 본다 (SFTP 처럼 만들기만 해서는 접속되는지 모르는 모드).
func verifyRemote(rc *rclone.Client, name string, progress func(string)) error {
	progress(i18n.T("저장소에 접속하는 중…"))
	if _, _, err := rc.LsDirs(config.Spec(name, config.BaseOf(rc, name), "")); err != nil {
		return fmt.Errorf(i18n.T("저장소에 접속하지 못했습니다: %v"), err)
	}
	return nil
}

// pendingSuffix 는 다시 연결하는 동안 새 연결에 붙이는 임시 이름 꼬리다.
const pendingSuffix = "-new"

// SwitchWait 는 새 연결로 바꾸기 전에 동기화가 끝나기를 기다리는 최대 시간이다.
var SwitchWait = 2 * time.Minute

// switchRemote 는 임시 이름(tmp)으로 새로 만든 연결을 확인하고 원래 연결(st.Remote) 자리에 넣는다.
//
// 이미 설정을 마친 PC 면 새 연결에서도 같은 setting.json 이 보이고 그 안에 이 PC 가 있어야 바꾼다.
// 다른 계정으로 로그인했거나, 새 클라이언트가 기존 폴더를 못 보는 범위(구글의 '이 앱이 만든 파일만')면
// 동기화가 저장소 쪽 파일이 모두 지워졌다고 보고 이 PC 의 파일을 지울 수 있기 때문이다.
func switchRemote(rc *rclone.Client, st store.Settings, tmp string, progress func(string)) error {
	if st.Configured {
		progress(i18n.T("새 연결로 GodseonSync/setting.json 을 확인하는 중…"))
		cfg, err := config.NewDrive(rc, tmp, config.RootFolder).Load()
		switch {
		case errors.Is(err, config.ErrNoSetting):
			return errors.New(i18n.T("새 연결에서 GodseonSync/setting.json 을 찾지 못해 바꾸지 않았습니다 (지금 연결은 그대로입니다). 같은 계정으로 로그인했는지 확인하세요. 구글 드라이브는 권한 범위가 '이 앱이 만든 파일만' 이면 다른 클라이언트로 만든 폴더가 보이지 않습니다."))
		case err != nil:
			return fmt.Errorf(i18n.T("새 연결로 저장소를 읽지 못해 바꾸지 않았습니다 (지금 연결은 그대로입니다): %v"), err)
		case cfg.FindPC(st.ClientID) == nil:
			return errors.New(i18n.T("새 연결의 setting.json 에 이 PC 가 없어 바꾸지 않았습니다 (지금 연결은 그대로입니다). 다른 계정의 GodseonSync 폴더일 수 있습니다."))
		}
	}
	// 동기화 중인 rclone 이 토큰을 갱신하며 rclone.conf 를 다시 쓰는 것과 겹치지 않게 끝나기를 기다린다
	for deadline := time.Now().Add(SwitchWait); store.ReadLock() != nil && time.Now().Before(deadline); {
		progress(i18n.T("지금 하는 동기화가 끝나기를 기다리는 중…"))
		time.Sleep(time.Second)
	}
	if err := rc.ReplaceRemote(tmp, st.Remote); err != nil {
		return err
	}
	store.Log("저장소 연결을 바꿨습니다 (%s)", st.Remote)
	store.AddNotice("ok", i18n.T("저장소 연결을 바꿨습니다"), "")
	return nil
}

func (s *Server) connectCancel(r *http.Request) (any, error) {
	s.mu.Lock()
	if s.job != nil {
		s.job.Cancel()
	}
	s.mu.Unlock()
	return map[string]any{"connect": s.connectState()}, nil
}

func (s *Server) disconnect(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	st := store.LoadSettings()
	if err := rclone.New().DeleteRemote(st.Remote); err != nil {
		return nil, err
	}
	st.Configured = false
	if err := store.SaveSettings(st); err != nil {
		return nil, err
	}
	s.ctrl.SettingsChanged()
	return map[string]bool{"ok": true}, nil
}

// probe 는 구글 로그인 직후 부른다. 드라이브에 GodseonSync 폴더와 setting.json 이 없으면 만들고,
// 이미 있으면 등록된 폴더와 PC 를 알려 준다 (이 PC 를 새로 등록할지, 기존 PC 를 이어받을지 고르게).
func (s *Server) probe(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	st := store.LoadSettings()
	rc := rclone.New()
	if _, _, err := rc.LsDirs(engine.NewDrive(st).Spec("")); err != nil {
		return nil, fail(http.StatusBadGateway, "구글 드라이브에 접속할 수 없습니다: %v", err)
	}
	d := engine.NewDrive(st)
	cfg, created, err := d.Ensure(st.ClientName, nil)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			// SFTP 서버가 홈 폴더 맨 위에 쓰기를 막은 경우 (chroot) — 어디를 고치면 되는지 알려 준다
			where := d.Base
			if where == "" {
				where = i18n.T("맨 위(홈) 폴더")
			}
			return nil, fail(http.StatusBadRequest, "저장소의 %s 에 GodseonSync 폴더를 만들 권한이 없습니다. 설정 › 모드(처음 설정은 저장소 연결)의 '폴더 위치' 에 쓸 수 있는 하위 폴더(예: upload)를 넣고 다시 연결하세요. (%v)", where, err)
		}
		return nil, err
	}
	statuses, _ := d.LoadStatuses()
	type pcInfo struct {
		config.PC
		LastSeen string `json:"last_seen,omitempty"`
	}
	pcs := []pcInfo{}
	for _, pc := range cfg.PCs {
		pcs = append(pcs, pcInfo{PC: pc, LastSeen: statuses[pc.ID].LastSeen})
	}
	names := []string{}
	for _, f := range cfg.Folders {
		names = append(names, f.Name)
	}
	return map[string]any{"created": created, "setting": d.SettingPath(), "folders": names, "pcs": pcs}, nil
}

func (s *Server) finishSetup(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	var body struct {
		ClientName string `json:"client_name"`
		Adopt      string `json:"adopt_client_id"`
		Autostart  bool   `json:"autostart"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(body.ClientName)
	if name == "" {
		name = platform.Hostname()
	}
	st := store.LoadSettings()
	st.Root, st.ClientName = config.RootFolder, name
	if body.Adopt != "" {
		st.ClientID = body.Adopt // 기존 PC 이어받기: 그 PC 의 경로 설정을 그대로 쓴다
	}
	d := engine.NewDrive(st)
	if _, _, err := d.Ensure(name, nil); err != nil {
		return nil, err
	}
	cfg, err := d.Update(name, func(c *config.Config) error {
		if body.Adopt != "" && c.FindPC(body.Adopt) == nil {
			return fail(http.StatusBadRequest, "이어받을 PC 가 목록에 없습니다. 다시 확인을 누르세요.")
		}
		pc := c.FindPC(st.ClientID)
		if pc == nil {
			c.PCs = append(c.PCs, config.PC{ID: st.ClientID, AddedAt: store.Now()})
			pc = &c.PCs[len(c.PCs)-1]
		}
		pc.Name, pc.OS = name, platform.OSKey
		return nil
	})
	if err != nil {
		return nil, err
	}
	st.Configured = true
	if err := store.SaveSettings(st); err != nil {
		return nil, err
	}
	_ = store.WriteJSON(store.CacheFile(), cfg)
	msg := ""
	if body.Autostart {
		if err := platform.SetAutostart(true); err != nil {
			msg = i18n.T("로그인 자동 실행을 켜지 못했습니다: %v", err)
		}
	}
	engine.PublishStatus(d, st)
	s.ctrl.SettingsChanged()
	return map[string]any{"ok": true, "message": msg}, nil
}

func (s *Server) reset(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	st := store.LoadSettings()
	st.Configured = false
	if err := store.SaveSettings(st); err != nil {
		return nil, err
	}
	s.ctrl.SettingsChanged()
	return map[string]bool{"ok": true}, nil
}

// ------------------------------------------------------------ 설정 바꾸기

// saveConfig 는 화면에서 고친 폴더 목록과 공통 제외 규칙을 저장한다. 화면을 연 뒤 다른 PC 가 폴더 목록을
// 바꿨으면 충돌로 알리고, PC 목록만 바뀌었으면(새 PC 등록 등) 그대로 합쳐서 저장한다.
func (s *Server) saveConfig(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	var body struct {
		Folders []config.Folder `json:"folders"`
		Exclude []string        `json:"exclude"`
		Base    config.Config   `json:"base"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	st := store.LoadSettings()
	d := engine.NewDrive(st)
	base := &body.Base
	if err := base.Normalize(); err != nil {
		return nil, err
	}
	saved, err := d.Update(st.ClientName, func(c *config.Config) error {
		if !config.SameFolders(c, base) {
			return &config.ConflictError{Current: c}
		}
		c.Folders, c.Exclude = body.Folders, body.Exclude
		return nil
	})
	if errors.Is(err, config.ErrNoSetting) {
		// 드라이브에 setting.json 이 없으면 지금 저장하려는 설정으로 새로 만든다
		c := &config.Config{Exclude: body.Exclude, Folders: body.Folders, PCs: base.PCs}
		if c.FindPC(st.ClientID) == nil {
			c.PCs = append(c.PCs, config.PC{ID: st.ClientID, Name: st.ClientName, OS: platform.OSKey, AddedAt: store.Now()})
		}
		if saved, err = d.Create(c, st.ClientName); err == nil {
			store.Log("드라이브에 %s 이 없어 지금 저장한 설정으로 새로 만들었습니다", d.SettingPath())
		}
	}
	var ce *config.ConflictError
	var ve *config.ValidationError
	switch {
	case errors.As(err, &ve):
		return nil, &apiError{status: http.StatusUnprocessableEntity, msg: i18n.T("저장하지 못했습니다."), extra: map[string]any{"errors": ve.Problems}}
	case errors.As(err, &ce):
		return nil, &apiError{status: http.StatusConflict, msg: ce.Error(), extra: map[string]any{"config": ce.Current}}
	case err != nil:
		return nil, fail(http.StatusBadGateway, "드라이브에 저장하지 못했습니다: %v", err)
	}
	_ = store.WriteJSON(store.CacheFile(), saved)
	return map[string]any{"config": saved, "warnings": warnings(saved, st)}, nil
}

func (s *Server) renameClient(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return nil, fail(http.StatusBadRequest, "이름을 입력하세요.")
	}
	st := store.LoadSettings()
	st.ClientName = name
	if err := store.SaveSettings(st); err != nil {
		return nil, err
	}
	d := engine.NewDrive(st)
	cfg, err := d.Load()
	if err == nil {
		_, err = engine.Register(d, cfg, st)
	}
	return map[string]bool{"ok": true}, err
}

func (s *Server) removeClient(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	st := store.LoadSettings()
	if body.ID == "" || body.ID == st.ClientID {
		return nil, fail(http.StatusBadRequest, "이 PC 는 목록에서 뺄 수 없습니다.")
	}
	d := engine.NewDrive(st)
	cfg, err := d.Update(st.ClientName, func(c *config.Config) error {
		pcs := c.PCs[:0]
		for _, pc := range c.PCs {
			if pc.ID != body.ID {
				pcs = append(pcs, pc)
			}
		}
		c.PCs = pcs
		for i := range c.Folders {
			delete(c.Folders[i].Paths, body.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := d.DeleteStatus(body.ID); err != nil {
		return nil, err
	}
	return map[string]any{"config": cfg}, nil
}

// ------------------------------------------------------------ 실행

func (s *Server) sync(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	var body struct {
		Folders []string `json:"folders"`
		Resync  bool     `json:"resync"`
		Force   bool     `json:"force"`
		DryRun  bool     `json:"dry_run"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	if err := s.ctrl.StartSync(engine.Options{Folders: body.Folders, Resync: body.Resync, Force: body.Force, DryRun: body.DryRun}); err != nil {
		return nil, fail(http.StatusConflict, "%v", err)
	}
	return map[string]bool{"started": true}, nil
}

// chooseFolder 는 양쪽에 파일이 있어 멈춘 폴더를 어느 쪽으로 맞출지 저장하고 그 폴더를 바로 동기화한다.
// choice: drive(드라이브 → 이 PC), local(이 PC → 드라이브), merge(합치기). 덮어쓰기 전 백업은 엔진이 한다.
func (s *Server) chooseFolder(r *http.Request) (any, error) {
	if s.migrating.Load() {
		return nil, errMigrating()
	}
	var body struct {
		ID     string `json:"id"`
		Choice string `json:"choice"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	switch body.Choice {
	case store.ChoiceDrive, store.ChoiceLocal, store.ChoiceMerge:
	default:
		return nil, fail(http.StatusBadRequest, "모르는 선택입니다: %s", body.Choice)
	}
	if body.ID == "" {
		return nil, fail(http.StatusBadRequest, "폴더를 고르세요.")
	}
	if err := store.SetChoice(body.ID, body.Choice); err != nil {
		return nil, err
	}
	if err := s.ctrl.StartSync(engine.Options{Folders: []string{body.ID}}); err != nil {
		return map[string]any{"saved": true, "started": false, "message": err.Error()}, nil
	}
	return map[string]any{"saved": true, "started": true}, nil
}

func (s *Server) background(r *http.Request) (any, error) {
	var body struct {
		Autostart *bool   `json:"autostart"`
		Interval  *int    `json:"interval_minutes"`
		Paused    *bool   `json:"paused"`
		Language  *string `json:"language"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	st := store.LoadSettings()
	if body.Interval != nil {
		if *body.Interval < 1 || *body.Interval > 1440 {
			return nil, fail(http.StatusBadRequest, "주기는 1~1440분 사이로 정하세요.")
		}
		st.IntervalMinutes = *body.Interval
	}
	if body.Paused != nil {
		st.Paused = *body.Paused
	}
	if body.Language != nil {
		if *body.Language != i18n.Korean && *body.Language != i18n.English {
			return nil, fail(http.StatusBadRequest, "모르는 언어입니다: %s", *body.Language)
		}
		st.Language = *body.Language
	}
	if err := store.SaveSettings(st); err != nil {
		return nil, err
	}
	if body.Autostart != nil {
		if err := platform.SetAutostart(*body.Autostart); err != nil {
			return nil, fail(http.StatusInternalServerError, "자동 실행 설정 실패: %v", err)
		}
	}
	s.ctrl.SettingsChanged()
	if st.Configured {
		go engine.PublishStatus(engine.NewDrive(st), st)
	}
	return map[string]any{"settings": st, "autostart": platform.AutostartEnabled()}, nil
}

func (s *Server) pickFolder(r *http.Request) (any, error) {
	var body struct {
		Initial string `json:"initial"`
	}
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	p, err := platform.PickFolder(body.Initial, i18n.T("동기화할 폴더를 고르세요"))
	if err != nil {
		return nil, err
	}
	return map[string]string{"path": p}, nil
}

func (s *Server) quit(r *http.Request) (any, error) {
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.ctrl.Quit()
	}()
	return map[string]bool{"ok": true}, nil
}
