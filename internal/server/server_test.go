package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/engine"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/rclone"
	"github.com/afy0817/GodseonSync/internal/rcloneembed"
	"github.com/afy0817/GodseonSync/internal/store"
)

// 테스트 바이너리도 `<바이너리> rclone ...` 으로 불리면 rclone 처럼 동작하게 한다.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "rclone" {
		rcloneembed.Main(os.Args[2:])
	}
	config.NotFoundRetry.Delay = 0
	os.Exit(m.Run())
}

type fakeCtrl struct{}

func (fakeCtrl) StartSync(engine.Options) error { return nil }
func (fakeCtrl) SettingsChanged()               {}
func (fakeCtrl) OpenUI()                        {}
func (fakeCtrl) Quit()                          {}

func newServer() *Server {
	s := New(fakeCtrl{})
	s.Port = 9417
	return s
}

// call 은 설정 화면이 부르듯 API 를 부르고 JSON 응답을 돌려준다.
func call(t *testing.T, s *Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Host = "127.0.0.1:9417"
	req.Header.Set("X-GS-Token", s.Token)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func home(t *testing.T, remote string) store.Settings {
	t.Helper()
	t.Setenv("GODSEONSYNC_HOME", t.TempDir())
	st := store.Settings{ClientID: "pc-a", ClientName: "맥", Remote: remote, Root: "GodseonSync", IntervalMinutes: 10, Configured: true}
	if err := store.SaveSettings(st); err != nil {
		t.Fatal(err)
	}
	return st
}

// 캐시로 열기는 드라이브(rclone)에 전혀 가지 않고 바로 답해야 한다.
func TestConfigCachedSkipsDrive(t *testing.T) {
	home(t, "nodrive") // 정의되지 않은 연결 → 드라이브에 가면 실패한다
	cached := config.Default()
	cached.Revision = 7
	cached.PCs = []config.PC{{ID: "pc-a", Name: "맥", OS: "mac"}}
	cached.Folders = []config.Folder{{ID: "f-1", Name: "문서", DrivePath: "GodseonSync/문서", Paths: map[string]string{"pc-a": "/tmp/x"}}}
	_ = store.WriteJSON(store.CacheFile(), cached)
	_ = store.WriteJSON(store.StatusesCacheFile(), map[string]config.Status{"pc-a": {ID: "pc-a", LastSeen: "2026-10-19T10:00:00+09:00"}})

	s := newServer()
	start := time.Now()
	code, out := call(t, s, "GET", "/api/config?cached=1", nil)
	if code != 200 || out["cached"] != true || out["cached_at"] == nil {
		t.Fatalf("캐시 응답이 와야 함: %d %v", code, out)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("캐시 응답은 바로 와야 함 (%s)", time.Since(start))
	}
	cfg := out["config"].(map[string]any)
	if cfg["revision"].(float64) != 7 || len(cfg["folders"].([]any)) != 1 {
		t.Fatalf("저장해 둔 설정이 와야 함: %v", cfg)
	}
	if _, ok := out["statuses"].(map[string]any)["pc-a"]; !ok {
		t.Fatalf("저장해 둔 PC 상태가 와야 함: %v", out["statuses"])
	}

	// 드라이브에서 받기가 실패하면 offline 과 함께 캐시를 준다
	code, out = call(t, s, "GET", "/api/config", nil)
	if code != 200 || out["offline"] != true || out["config"] == nil {
		t.Fatalf("드라이브 실패 시 캐시와 offline 이 와야 함: %d %v", code, out)
	}
}

func fakeDrive(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("RCLONE_CONFIG_FAKE_TYPE", "alias")
	t.Setenv("RCLONE_CONFIG_FAKE_REMOTE", dir)
	return dir
}

// 드라이브에서 받으면 설정·PC 상태 캐시를 갱신하고, 편집을 시작한 시점의 설정(base)으로 충돌을 가린다.
func TestFreshConfigAndStaleBase(t *testing.T) {
	drive := fakeDrive(t)
	st := home(t, "fake")
	s := newServer()

	code, out := call(t, s, "GET", "/api/config", nil)
	if code != 200 || out["offline"] != false || out["synced_at"] == nil {
		t.Fatalf("드라이브에서 받아야 함: %d %v", code, out)
	}
	if _, err := os.Stat(store.StatusesCacheFile()); err != nil {
		t.Fatal("PC 상태 캐시를 저장해야 함")
	}
	if _, err := os.Stat(filepath.Join(drive, "GodseonSync", "setting.json")); err != nil {
		t.Fatal("드라이브에 setting.json 이 있어야 함")
	}
	d := engine.NewDrive(st)
	base, err := d.Load() // 화면이 편집을 시작한 시점의 설정
	if err != nil {
		t.Fatal(err)
	}
	edit := []config.Folder{{Name: "내 폴더", DrivePath: "GodseonSync/mine", Paths: map[string]string{"pc-a": filepath.Join(t.TempDir(), "mine")}}} // 이 PC 의 OS 에 맞는 전체 경로

	// 그사이 다른 PC 는 등록만 했다 → 폴더 목록이 같으니 합쳐서 저장된다
	if _, err := d.Update("다른 PC", func(c *config.Config) error {
		c.PCs = append(c.PCs, config.PC{ID: "pc-b", Name: "윈도우", OS: "windows"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, out = call(t, s, "POST", "/api/config/save", map[string]any{"folders": edit, "exclude": base.Exclude, "base": base})
	if code != 200 {
		t.Fatalf("PC 등록만 바뀐 경우는 저장돼야 함: %d %v", code, out)
	}
	saved, _ := d.Load()
	if saved.FindPC("pc-b") == nil || len(saved.Folders) != 1 {
		t.Fatalf("다른 PC 등록과 내 폴더가 둘 다 남아야 함: %+v", saved)
	}

	// 같은 오래된 base 로 또 저장하면 (그사이 폴더 목록이 바뀌었으므로) 충돌로 막혀야 한다
	edit2 := []config.Folder{{Name: "덮어쓰기", DrivePath: "GodseonSync/other", Paths: map[string]string{"pc-a": "/tmp/other"}}}
	code, out = call(t, s, "POST", "/api/config/save", map[string]any{"folders": edit2, "exclude": base.Exclude, "base": base})
	if code != http.StatusConflict {
		t.Fatalf("오래된 기준으로 저장하면 409 여야 함: %d %v", code, out)
	}
	after, _ := d.Load()
	if len(after.Folders) != 1 || after.Folders[0].Name != "내 폴더" {
		t.Fatalf("충돌이면 드라이브 설정을 덮어쓰면 안 됨: %+v", after.Folders)
	}
}

// 잘못된 클라이언트 ID·보안 비밀은 rclone 을 띄우기 전에 거절한다.
func TestConnectRejectsBadClient(t *testing.T) {
	home(t, "gdrive")
	s := newServer()
	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{map[string]any{"mode": "gdrive", "options": map[string]string{"client_id": "abc", "client_secret": "x"}}, "apps.googleusercontent.com"},
		{map[string]any{"mode": "gdrive", "options": map[string]string{"client_id": "1-a.apps.googleusercontent.com"}}, "보안 비밀도"},
		{map[string]any{"mode": "gdrive", "options": map[string]string{"client_secret": "x"}}, "클라이언트 ID 도"},
		{map[string]any{"mode": "ftp"}, "모르는 저장소 방식"},
	} {
		code, out := call(t, s, "POST", "/api/connect", c.body)
		if msg, _ := out["error"].(string); code != http.StatusBadRequest || !strings.Contains(msg, c.want) {
			t.Fatalf("%v → %d %v (원하는 말: %s)", c.body, code, out, c.want)
		}
	}
	if s.job != nil {
		t.Fatal("연결 작업을 시작하면 안 됨")
	}
}

// writeSetting 은 가짜 저장소 dir 에 setting.json 을 만든다.
func writeSetting(t *testing.T, dir string, pcs ...string) {
	t.Helper()
	c := config.Default()
	c.Revision = 1
	for _, id := range pcs {
		c.PCs = append(c.PCs, config.PC{ID: id, Name: id, OS: "mac"})
	}
	if err := store.WriteJSON(filepath.Join(dir, "GodseonSync", "setting.json"), c); err != nil {
		t.Fatal(err)
	}
}

// 다시 연결: 새 연결(임시 이름)에서 같은 setting.json 과 이 PC 가 보일 때만 원래 연결과 바꾼다.
func TestReconnectSwitchesOnlyWhenSettingVisible(t *testing.T) {
	st := home(t, "gdrive")
	oldDir, newDir := t.TempDir(), t.TempDir()
	writeSetting(t, oldDir, "pc-a")
	conf := "[gdrive]\ntype = alias\nremote = " + oldDir + "\n"
	if err := os.WriteFile(store.RcloneConfig(), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	rc := rclone.New()
	remoteDir := func(name string) string { o, _ := rc.RemoteOptions(name); return o["remote"] }
	connect := func() map[string]string {
		t.Helper()
		job, err := rc.StartConnect("gdrive-new", "alias", []string{"remote=" + newDir}, rclone.ConnectHooks{
			Finish:  func(p func(string)) error { return switchRemote(rc, st, "gdrive-new", p) },
			Cleanup: func() { _ = rc.DeleteRemote("gdrive-new") },
		})
		if err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if snap := job.Snapshot(); snap["state"] != "running" {
				for i := 0; i < 100 && rc.HasRemote("gdrive-new"); i++ {
					time.Sleep(50 * time.Millisecond) // 실패하면 정리가 뒤에서 돈다
				}
				return snap
			}
		}
		t.Fatal("연결 작업이 끝나지 않음")
		return nil
	}

	// 1. 새 연결에 setting.json 이 없다 (다른 계정, 또는 '이 앱이 만든 파일만' 범위)
	if snap := connect(); snap["state"] != "error" || !strings.Contains(snap["message"], "찾지 못해") {
		t.Fatalf("setting.json 이 없으면 실패해야 함: %v", snap)
	}
	if remoteDir("gdrive") != oldDir || rc.HasRemote("gdrive-new") {
		t.Fatal("실패하면 지금 연결은 그대로, 임시 연결은 지워야 함")
	}
	// 2. setting.json 은 있는데 이 PC 가 없다 (다른 계정의 GodseonSync 폴더)
	writeSetting(t, newDir, "pc-b")
	if snap := connect(); snap["state"] != "error" || !strings.Contains(snap["message"], "이 PC 가 없어") {
		t.Fatalf("이 PC 가 없으면 실패해야 함: %v", snap)
	}
	if remoteDir("gdrive") != oldDir {
		t.Fatal("지금 연결은 그대로여야 함")
	}
	// 3. 같은 설정 → 바꾼다
	writeSetting(t, newDir, "pc-a", "pc-b")
	if snap := connect(); snap["state"] != "done" {
		t.Fatalf("바꿔야 함: %v", snap)
	}
	if remoteDir("gdrive") != newDir || rc.HasRemote("gdrive-new") {
		t.Fatalf("새 연결이 원래 이름에 들어가야 함: %s", remoteDir("gdrive"))
	}
}

// runConnect 는 가짜 저장소(alias)로 연결 작업을 돌리고 끝난 상태를 돌려준다 (실패하면 정리가 끝날 때까지 기다린다).
func runConnect(t *testing.T, rc *rclone.Client, dir string, finish func(func(string)) error) map[string]string {
	t.Helper()
	job, err := rc.StartConnect("gdrive-new", "alias", []string{"remote=" + dir}, rclone.ConnectHooks{
		Finish:  finish,
		Cleanup: func() { _ = rc.DeleteRemote("gdrive-new") },
	})
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if snap := job.Snapshot(); snap["state"] != "running" {
			return snap
		}
	}
	t.Fatal("연결 작업이 끝나지 않음")
	return nil
}

func readSetting(t *testing.T, p string) *config.Config {
	t.Helper()
	var c config.Config
	if err := store.ReadJSON(p, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

// 범위 옮기기: 옛 폴더는 이름만 바꿔 남기고, 새 연결에 새 세대·이어진 번호로 설정을 만든 뒤 연결을 바꾼다.
func TestMigrateScope(t *testing.T) {
	st := home(t, "gdrive")
	oldDir, newDir, local := t.TempDir(), t.TempDir(), t.TempDir()
	c := config.Default()
	c.Revision = 5
	c.PCs = []config.PC{{ID: "pc-a", Name: "맥", OS: platform.OSKey}} // 경로 검사는 PC 의 OS 기준이라 지금 OS 로
	c.Folders = []config.Folder{{ID: "f-1", Name: "문서", DrivePath: "GodseonSync/문서", Paths: map[string]string{"pc-a": local}}}
	if err := store.WriteJSON(filepath.Join(oldDir, "GodseonSync", "setting.json"), c); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(oldDir, "GodseonSync", "문서"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "GodseonSync", "문서", "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.RcloneConfig(), []byte("[gdrive]\ntype = alias\nremote = "+oldDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, rc := newServer(), rclone.New()

	snap := runConnect(t, rc, newDir, func(p func(string)) error { return s.migrateScope(rc, st, "gdrive-new", p) })
	if snap["state"] != "done" {
		t.Fatalf("옮겨야 함: %v", snap)
	}
	// 옛 폴더: 이름만 바뀌어 그대로 남는다
	if _, err := os.Stat(filepath.Join(oldDir, "GodseonSync")); !os.IsNotExist(err) {
		t.Fatal("옛 GodseonSync 는 이름이 바뀌어야 함")
	}
	backups, _ := filepath.Glob(filepath.Join(oldDir, "GodseonSync (옮기기 전 *)", "문서", "a.txt"))
	if len(backups) != 1 {
		t.Fatal("옛 폴더가 파일과 함께 '옮기기 전' 이름으로 남아야 함")
	}
	// 새 폴더: 같은 설정 + 새 세대 + 이어진 번호, 임시 폴더는 없음
	nc := readSetting(t, filepath.Join(newDir, "GodseonSync", "setting.json"))
	if nc.Epoch == "" || nc.Revision != 6 || nc.FindPC("pc-a") == nil || len(nc.Folders) != 1 || nc.Folders[0].DrivePath != "GodseonSync/문서" {
		t.Fatalf("새 setting.json: 세대=%q 번호=%d PC=%v 폴더=%v", nc.Epoch, nc.Revision, nc.PCs, nc.Folders)
	}
	if _, err := os.Stat(filepath.Join(newDir, movingRoot)); !os.IsNotExist(err) {
		t.Fatal("임시 폴더가 남으면 안 됨")
	}
	// 연결: 원래 이름이 새 저장소를 가리키고, 임시 연결·잠금·옮기는 중 표시는 없다
	if o, _ := rc.RemoteOptions("gdrive"); o["remote"] != newDir || rc.HasRemote("gdrive-new") {
		t.Fatalf("연결이 바뀌어야 함: %v", o)
	}
	if store.ReadLock() != nil || s.migrating.Load() {
		t.Fatal("잠금과 옮기는 중 표시가 풀려야 함")
	}

	// 다른 PC 가 먼저 옮겼으면(새 연결에 이 PC 가 든 setting.json) 바꿔 넣기만 한다
	otherDir := t.TempDir()
	writeSetting(t, otherDir, "pc-a")
	if snap := runConnect(t, rc, otherDir, func(p func(string)) error { return s.migrateScope(rc, st, "gdrive-new", p) }); snap["state"] != "done" {
		t.Fatalf("먼저 옮긴 폴더로 바꿔야 함: %v", snap)
	}
	if o, _ := rc.RemoteOptions("gdrive"); o["remote"] != otherDir {
		t.Fatal("먼저 옮긴 폴더로 바뀌어야 함")
	}
	if _, err := os.Stat(filepath.Join(newDir, "GodseonSync", "setting.json")); err != nil {
		t.Fatal("이 경우에는 지금 저장소를 건드리지 않아야 함")
	}
}

// 옮기는 동안: 설정 받기는 캐시로 답하고(드라이브의 setting.json 을 새로 만들지 않게), 설정 바꾸기는 막는다.
func TestMigratingBlocksChanges(t *testing.T) {
	home(t, "nodrive")
	_ = store.WriteJSON(store.CacheFile(), config.Default())
	s := newServer()
	s.migrating.Store(true)
	if code, out := call(t, s, "GET", "/api/config", nil); code != http.StatusOK || out["cached"] != true || out["offline"] != true {
		t.Fatalf("캐시로 답해야 함: %d %v", code, out)
	}
	for _, path := range []string{"/api/config/save", "/api/sync", "/api/client/rename", "/api/connect"} {
		if code, out := call(t, s, "POST", path, map[string]any{}); code != http.StatusConflict {
			t.Fatalf("%s 는 막아야 함: %d %v", path, code, out)
		}
	}
}

// 옮기기 전에 동기화해 둔 PC 가 옮긴 뒤 동기화하면: 새 폴더가 비어 있어도 '지워졌다' 로 보지 않고
// 세대가 바뀌어 합치기(지우지 않음)로 다시 올린다.
func TestMigrateThenSyncResyncs(t *testing.T) {
	st := home(t, "gdrive")
	oldDir, newDir, local := t.TempDir(), t.TempDir(), t.TempDir()
	c := config.Default()
	c.PCs = []config.PC{{ID: "pc-a", Name: "맥", OS: platform.OSKey}}
	c.Folders = []config.Folder{{ID: "f-1", Name: "문서", DrivePath: "GodseonSync/문서", Paths: map[string]string{"pc-a": local}}}
	if err := store.WriteJSON(filepath.Join(oldDir, "GodseonSync", "setting.json"), c); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(oldDir, "GodseonSync", "문서"), 0o755)
	_ = os.WriteFile(filepath.Join(oldDir, "GodseonSync", "문서", "드라이브에만.txt"), []byte("d"), 0o644)
	_ = os.WriteFile(filepath.Join(local, "이PC에만.txt"), []byte("l"), 0o644)
	if err := os.WriteFile(store.RcloneConfig(), []byte("[gdrive]\ntype = alias\nremote = "+oldDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Run(engine.Options{Resync: true}); err != nil { // 옮기기 전 첫 동기화: 합치기 (기록이 생긴다)
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(local, "드라이브에만.txt")); err != nil {
		t.Fatal("옮기기 전 동기화가 드라이브 파일을 받아야 함")
	}

	s, rc := newServer(), rclone.New()
	if snap := runConnect(t, rc, newDir, func(p func(string)) error { return s.migrateScope(rc, st, "gdrive-new", p) }); snap["state"] != "done" {
		t.Fatalf("옮겨야 함: %v", snap)
	}
	if _, err := engine.Run(engine.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"드라이브에만.txt", "이PC에만.txt"} {
		if _, err := os.Stat(filepath.Join(local, name)); err != nil {
			t.Fatalf("이 PC 의 %s 가 지워지면 안 됨", name)
		}
		if _, err := os.Stat(filepath.Join(newDir, "GodseonSync", "문서", name)); err != nil {
			t.Fatalf("새 폴더에 %s 가 다시 올라가야 함", name)
		}
	}
	if !strings.Contains(store.LogTail("", 200), "--resync") && !strings.Contains(store.LogTail("", 200), "합치기") {
		t.Log(store.LogTail("", 60))
		t.Fatal("옮긴 뒤 첫 동기화는 합치기여야 함")
	}
}

// 로그 API: 전체·폴더 로그를 따로 읽고 지운다. 파일 이름으로 못 쓰는 폴더 ID 는 거절한다.
func TestLogAPI(t *testing.T) {
	home(t, "nodrive")
	s := newServer()
	store.Log("GodseonSync 시작 (설정 화면 포트 %d)", 9417)
	store.FolderLog{ID: "f-1", Name: "문서"}.Both("✓ 완료 (%s): %s", "1초", "바뀐 파일 없음")
	store.FolderLog{ID: "f-1", Name: "문서"}.Log("  ↑ 올림%s: %s", "", "a.txt")
	_, all := call(t, s, "GET", "/api/log?lines=50", nil)
	_, one := call(t, s, "GET", "/api/log?lines=50&folder=f-1", nil)
	if txt := all["text"].(string); !strings.Contains(txt, "[문서] ✓ 완료") || strings.Contains(txt, "a.txt") {
		t.Fatalf("전체 로그: %v", all)
	}
	if txt := one["text"].(string); !strings.Contains(txt, "a.txt") || strings.Contains(txt, "시작") || !strings.HasSuffix(one["path"].(string), filepath.Join("logs", "f-1.log")) {
		t.Fatalf("폴더 로그: %v", one)
	}
	if fs, _ := all["folders"].([]any); len(fs) != 1 || fs[0] != "f-1" {
		t.Fatalf("로그가 있는 폴더: %v", all["folders"])
	}
	if code, _ := call(t, s, "GET", "/api/log?folder=..%2Fsettings", nil); code != http.StatusBadRequest {
		t.Fatal("이상한 폴더 ID 는 거절")
	}
	if code, out := call(t, s, "POST", "/api/log/clear", map[string]any{"folder": "f-1"}); code != http.StatusOK || strings.Contains(out["text"].(string), "a.txt") {
		t.Fatalf("폴더 로그 지우기: %d %v", code, out)
	}
	if _, all = call(t, s, "GET", "/api/log", nil); !strings.Contains(all["text"].(string), "[문서] ✓ 완료") {
		t.Fatal("폴더 로그를 지워도 전체 로그는 그대로")
	}
	if code, _ := call(t, s, "POST", "/api/log/clear", map[string]any{}); code != http.StatusOK || store.LogTail("f-1", 10) != "" || strings.Contains(store.LogTail("", 10), "시작") {
		t.Fatal("전체 지우기는 폴더 로그까지")
	}
}

// 결정 API: 값을 검사하고, 고른 값을 저장한 뒤 그 폴더만 동기화를 시작한다.
func TestChooseFolder(t *testing.T) {
	home(t, "nodrive")
	s := newServer()
	if code, _ := call(t, s, "POST", "/api/folder/choose", map[string]any{"id": "f-1", "choice": "모두지우기"}); code != http.StatusBadRequest {
		t.Fatal("모르는 값은 거절해야 함")
	}
	code, out := call(t, s, "POST", "/api/folder/choose", map[string]any{"id": "f-1", "choice": "local"})
	if code != http.StatusOK || out["started"] != true || store.Choice("f-1") != store.ChoiceLocal {
		t.Fatalf("저장하고 시작해야 함: %d %v", code, out)
	}
}

// 언어 바꾸기: 저장하고, 서버 오류 문구도 그 언어로 나온다.
func TestLanguageSetting(t *testing.T) {
	home(t, "nodrive")
	s := newServer()
	t.Cleanup(func() { _ = store.SaveSettings(store.Settings{}) })
	if code, _ := call(t, s, "POST", "/api/background", map[string]any{"language": "fr"}); code != http.StatusBadRequest {
		t.Fatal("모르는 언어는 거절")
	}
	if code, out := call(t, s, "POST", "/api/background", map[string]any{"language": "en"}); code != http.StatusOK || store.LoadSettings().Language != "en" {
		t.Fatalf("영어로 저장: %d %v", code, out)
	}
	if _, out := call(t, s, "POST", "/api/folder/choose", map[string]any{"id": "f-1", "choice": "x"}); out["error"] != "Unknown choice: x" {
		t.Fatalf("오류 문구가 영어여야 함: %v", out)
	}
	call(t, s, "POST", "/api/background", map[string]any{"language": "ko"})
	if _, out := call(t, s, "POST", "/api/folder/choose", map[string]any{"id": "f-1", "choice": "x"}); out["error"] != "모르는 선택입니다: x" {
		t.Fatalf("다시 한국어: %v", out)
	}
}

// 알림 API: 남기기·목록·읽음·삭제, 상태에 안 읽은 수.
func TestNoticesAPI(t *testing.T) {
	home(t, "nodrive")
	s := newServer()
	if _, out := call(t, s, "GET", "/api/notices", nil); out["items"] == nil {
		t.Fatalf("알림이 없으면 빈 목록이어야 함 (null 아님): %v", out)
	}
	_, out := call(t, s, "POST", "/api/notices", map[string]any{"level": "err", "title": "화면 오류"})
	if out["unread"] != float64(1) {
		t.Fatalf("남기기: %v", out)
	}
	if _, st := call(t, s, "GET", "/api/status", nil); st["unread"] != float64(1) {
		t.Fatalf("상태에 안 읽은 수: %v", st)
	}
	_, out = call(t, s, "POST", "/api/notices/read", map[string]any{"all": true})
	if out["unread"] != float64(0) {
		t.Fatalf("모두 읽음: %v", out)
	}
	_, out = call(t, s, "POST", "/api/notices/delete", map[string]any{"all": true})
	if items, _ := out["items"].([]any); len(items) != 0 {
		t.Fatalf("모두 지우기: %v", out)
	}
}

// 데이터 폴더를 따로 쓰는 실행(테스트·시험용)은 이 PC 의 로그인 항목을 건드리지 않는다.
func TestSeparateHomeNeverTouchesAutostart(t *testing.T) {
	home(t, "nodrive")
	if !platform.SeparateHome() {
		t.Fatal("GODSEONSYNC_HOME 이 있으면 따로 쓰는 실행")
	}
	if err := platform.SetAutostart(true); err != nil {
		t.Fatal(err)
	}
}
