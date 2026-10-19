package server

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/rclone/rclone/cmd/serve/sftp" // 테스트 바이너리 안에서 진짜 SFTP 서버를 띄운다 (`rclone serve sftp`)

	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/engine"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/rclone"
	"github.com/afy0817/GodseonSync/internal/store"
)

// startSFTP 는 dir 을 홈 폴더로 쓰는 SFTP 서버를 띄우고 포트를 돌려준다 (사용자 gs, 비밀번호 pw).
func startSFTP(t *testing.T, dir string) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	cmd := exec.Command(os.Args[0], "rclone", "serve", "sftp", dir, "--addr", "127.0.0.1:"+port, "--user", "gs", "--pass", "pw")
	cmd.Env = append(os.Environ(), "RCLONE_CACHE_DIR="+t.TempDir(), "RCLONE_CONFIG="+filepath.Join(t.TempDir(), "none.conf"))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if c, err := net.Dial("tcp", "127.0.0.1:"+port); err == nil {
			c.Close()
			return port
		}
	}
	t.Fatal("SFTP 서버가 뜨지 않음")
	return ""
}

func waitJob(t *testing.T, s *Server) map[string]string {
	t.Helper()
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		s.mu.Lock()
		job := s.job
		s.mu.Unlock()
		if snap := job.Snapshot(); snap["state"] != "running" {
			return snap
		}
	}
	t.Fatal("연결 작업이 끝나지 않음")
	return nil
}

// 처음 설정에서 SFTP 로 연결: 실제로 접속되는지 확인하고, 비밀번호는 가려서 저장한다. 틀리면 연결을 지운다.
func TestSFTPFirstConnect(t *testing.T) {
	t.Setenv("GODSEONSYNC_HOME", t.TempDir())
	_ = store.SaveSettings(store.Settings{ClientID: "pc-a", ClientName: "맥", Remote: "gdrive", IntervalMinutes: 10})
	port := startSFTP(t, t.TempDir())
	s, rc := newServer(), rclone.New()

	opts := map[string]string{"host": "127.0.0.1", "port": port, "user": "gs", "auth": "password", "pass": "틀림"}
	if code, out := call(t, s, "POST", "/api/connect", map[string]any{"mode": "sftp", "options": opts}); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if snap := waitJob(t, s); snap["state"] != "error" || !strings.Contains(snap["message"], "접속하지 못했습니다") || rc.HasRemote("gdrive") {
		t.Fatalf("비밀번호가 틀리면 실패하고 연결을 지워야 함: %v", snap)
	}

	opts["pass"] = "pw"
	call(t, s, "POST", "/api/connect", map[string]any{"mode": "sftp", "options": opts})
	if snap := waitJob(t, s); snap["state"] != "done" {
		t.Fatalf("연결돼야 함: %v", snap)
	}
	o, _ := rc.RemoteOptions("gdrive")
	if o["type"] != "sftp" || o["host"] != "127.0.0.1" || o["user"] != "gs" || o["pass"] == "" || o["pass"] == "pw" {
		t.Fatalf("SFTP 연결, 비밀번호는 가려서 저장: type=%s host=%s user=%s 가림=%v", o["type"], o["host"], o["user"], o["pass"] != "pw")
	}
	_, ov := call(t, s, "GET", "/api/overview", nil)
	conn, _ := ov["connection"].(map[string]any)
	if conn["mode"] != "sftp" || conn["where"] != "gs@127.0.0.1:"+port || strings.Contains(fmt.Sprint(conn), "pw") {
		t.Fatalf("화면에는 위치만 (비밀번호 없음): %v", conn)
	}
}

// 쓰던 PC 를 SFTP 로 바꾸기: 새 저장소에 새 세대로 설정을 만들고 파일은 합치며 다시 올린다.
// 지금까지 쓰던 저장소(setting.json·파일)는 그대로, 연결은 gs-saved-<모드> 로 저장해 둔다.
func TestMoveStorageToSFTP(t *testing.T) {
	home(t, "gdrive")
	oldDir, sftpDir, local := t.TempDir(), t.TempDir(), t.TempDir()
	c := config.Default()
	c.Revision = 4
	// 이 PC 의 OS 는 지금 도는 OS 로 둔다. 다르면 바꾸기 전 동기화가 OS 를 고쳐 쓰며 리비전을 하나 더 올린다 (리눅스 CI 에서 실패했던 이유).
	c.PCs = []config.PC{{ID: "pc-a", Name: "맥", OS: platform.OSKey}, {ID: "pc-b", Name: "윈도우", OS: "windows"}}
	c.Folders = []config.Folder{{ID: "f-1", Name: "문서", DrivePath: "GodseonSync/문서", Paths: map[string]string{"pc-a": local}}}
	oldSetting := filepath.Join(oldDir, "GodseonSync", "setting.json")
	if err := store.WriteJSON(oldSetting, c); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(local, "a.txt"), []byte("A"), 0o644)
	if err := os.WriteFile(store.RcloneConfig(), []byte("[gdrive]\ntype = alias\nremote = "+oldDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Run(engine.Options{}); err != nil { // 바꾸기 전 동기화 (지금 저장소)
		t.Fatal(err)
	}
	before, _ := os.ReadFile(oldSetting)
	port := startSFTP(t, sftpDir)
	s := newServer()
	opts := map[string]string{"host": "127.0.0.1", "port": port, "user": "gs", "pass": "pw"}
	if code, out := call(t, s, "POST", "/api/connect", map[string]any{"mode": "sftp", "options": opts, "move": true}); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if snap := waitJob(t, s); snap["state"] != "done" {
		t.Fatalf("바꿔야 함: %v\n%s", snap, store.LogTail(30))
	}
	nc := readSetting(t, filepath.Join(sftpDir, "GodseonSync", "setting.json"))
	if nc.Epoch == "" || nc.Revision != 5 || len(nc.PCs) != 2 || len(nc.Folders) != 1 {
		t.Fatalf("새 저장소 설정: %+v", nc)
	}
	if after, _ := os.ReadFile(oldSetting); string(after) != string(before) {
		t.Fatal("지금까지 쓰던 저장소의 setting.json 은 그대로여야 함")
	}
	rc := rclone.New()
	if o, _ := rc.RemoteOptions("gdrive"); o["type"] != "sftp" {
		t.Fatalf("연결이 SFTP 로 바뀌어야 함: %v", o["type"])
	}
	if o, ok := rc.RemoteOptions("gs-saved-alias"); !ok || o["remote"] != oldDir {
		t.Fatalf("지금까지 쓰던 연결을 저장해 두어야 함: %v", o)
	}
	if _, err := engine.Run(engine.Options{}); err != nil { // 새 세대라 합치며 SFTP 로 올린다
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(sftpDir, "GodseonSync", "문서", "a.txt")); err != nil || string(b) != "A" {
		t.Fatalf("SFTP 로 다시 올라가야 함: %v\n%s", err, store.LogTail(30))
	}
}

// 저장해 둔 연결로 돌아가기: 로그인 없이 그 저장소의 마지막 설정으로 연결하고, 지금 연결을 저장한다 (SFTP 서버 두 대로 왕복).
func TestRestoreSavedConnection(t *testing.T) {
	t.Setenv("GODSEONSYNC_HOME", t.TempDir())
	_ = store.SaveSettings(store.Settings{ClientID: "pc-a", ClientName: "맥", Remote: "gdrive", IntervalMinutes: 10})
	dirA, dirB, local := t.TempDir(), t.TempDir(), t.TempDir()
	portA, portB := startSFTP(t, dirA), startSFTP(t, dirB)
	s, rc := newServer(), rclone.New()
	call(t, s, "POST", "/api/connect", map[string]any{"mode": "sftp", "options": map[string]string{"host": "127.0.0.1", "port": portA, "user": "gs", "pass": "pw"}})
	if snap := waitJob(t, s); snap["state"] != "done" {
		t.Fatalf("A 연결: %v", snap)
	}
	call(t, s, "POST", "/api/setup/probe", map[string]any{})
	if code, out := call(t, s, "POST", "/api/setup/finish", map[string]any{"client_name": "맥", "autostart": false}); code != 200 {
		t.Fatalf("처음 설정: %d %v", code, out)
	}
	settingA := filepath.Join(dirA, "GodseonSync", "setting.json")
	d := config.NewDrive(rc, "gdrive", "")
	if _, err := d.Update("맥", func(c *config.Config) error {
		c.Folders = []config.Folder{{ID: "f-1", Name: "문서", DrivePath: "GodseonSync/문서", Paths: map[string]string{"pc-a": local}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	beforeA, _ := os.ReadFile(settingA)

	// A → B (새로 연결)
	call(t, s, "POST", "/api/connect", map[string]any{"mode": "sftp", "options": map[string]string{"host": "127.0.0.1", "port": portB, "user": "gs", "pass": "pw"}, "move": true})
	if snap := waitJob(t, s); snap["state"] != "done" {
		t.Fatalf("B 로 바꾸기: %v\n%s", snap, store.LogTail(20))
	}
	if o, _ := rc.RemoteOptions("gdrive"); o["port"] != portB {
		t.Fatal("B 에 연결돼야 함")
	}
	if o, _ := rc.RemoteOptions("gs-saved-sftp"); o["port"] != portA {
		t.Fatal("A 연결을 저장해 두어야 함")
	}
	_, ov := call(t, s, "GET", "/api/overview", nil)
	saved, _ := ov["saved"].(map[string]any)
	if sv, _ := saved["sftp"].(map[string]any); sv["where"] != "gs@127.0.0.1:"+portA {
		t.Fatalf("화면에 저장된 연결: %v", saved)
	}

	// 저장해 둔 A 로 돌아가기 (로그인 없음): A 의 마지막 설정 그대로, B 연결을 저장
	if code, out := call(t, s, "POST", "/api/connect", map[string]any{"mode": "sftp", "restore": true}); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	if snap := waitJob(t, s); snap["state"] != "done" {
		t.Fatalf("A 로 돌아가기: %v\n%s", snap, store.LogTail(20))
	}
	if o, _ := rc.RemoteOptions("gdrive"); o["port"] != portA {
		t.Fatal("A 에 다시 연결돼야 함")
	}
	if o, _ := rc.RemoteOptions("gs-saved-sftp"); o["port"] != portB {
		t.Fatal("이번엔 B 연결을 저장해 두어야 함")
	}
	if afterA, _ := os.ReadFile(settingA); string(afterA) != string(beforeA) {
		t.Fatal("돌아온 A 의 setting.json 은 마지막 그대로여야 함")
	}
	if rc.HasRemote("gs-swap") || rc.HasRemote("gdrive-new") {
		t.Fatal("임시 연결이 남으면 안 됨")
	}
	// 저장된 연결이 없는 모드로 돌아가기는 거절
	if code, _ := call(t, s, "POST", "/api/connect", map[string]any{"mode": "gdrive", "restore": true}); code != 400 {
		t.Fatal("저장된 구글 연결이 없으면 거절")
	}
}

// 기본 위치(폴더): 홈 폴더 맨 위에 쓸 수 없는 서버를 위해 그 아래에 GodseonSync 를 만든다.
func TestSFTPBasePath(t *testing.T) {
	t.Setenv("GODSEONSYNC_HOME", t.TempDir())
	_ = store.SaveSettings(store.Settings{ClientID: "pc-a", ClientName: "맥", Remote: "gdrive", IntervalMinutes: 10})
	dir := t.TempDir()
	port := startSFTP(t, dir)
	s := newServer()
	call(t, s, "POST", "/api/connect", map[string]any{"mode": "sftp", "options": map[string]string{"host": "127.0.0.1", "port": port, "user": "gs", "pass": "pw", "path": "sub/inner"}})
	if snap := waitJob(t, s); snap["state"] != "done" {
		t.Fatalf("연결돼야 함: %v", snap)
	}
	if o, _ := rclone.New().RemoteOptions("gdrive"); o["gs_path"] != "sub/inner" {
		t.Fatalf("폴더는 연결 칸의 gs_path: %v", o["gs_path"])
	}
	if code, out := call(t, s, "POST", "/api/setup/probe", map[string]any{}); code != 200 || out["created"] != true {
		t.Fatalf("GodseonSync 를 만들어야 함: %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "inner", "GodseonSync", "setting.json")); err != nil {
		t.Fatalf("폴더 아래에 만들어야 함: %v", err)
	}
	_, ov := call(t, s, "GET", "/api/overview", nil)
	if conn, _ := ov["connection"].(map[string]any); conn["where"] != "gs@127.0.0.1:"+port+"/sub/inner" {
		t.Fatalf("위치: %v", conn["where"])
	}
}
