package storage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/afy0817/GodseonSync/internal/rclone"
)

func TestGoogleParams(t *testing.T) {
	got, err := GoogleDrive.Params(map[string]string{"scope": "drive.file", "client_id": " 1-a.apps.googleusercontent.com ", "client_secret": "s"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"scope=drive.file", "client_id=1-a.apps.googleusercontent.com", "client_secret=s"}; !slices.Equal(got, want) {
		t.Fatalf("%v", got)
	}
	if got, _ := GoogleDrive.Params(map[string]string{"scope": "이상한값"}); !slices.Equal(got, []string{"scope=drive.file"}) {
		t.Fatalf("기본 범위는 drive.file, 클라이언트를 비우면 공용 클라이언트여야 함: %v", got)
	}
	if got, _ := GoogleDrive.Params(map[string]string{"scope": "drive"}); !slices.Equal(got, []string{"scope=drive"}) {
		t.Fatalf("drive 는 고른 때만: %v", got)
	}
}

// 범위 옮기기: 화면에서 넣은 클라이언트 → (빌드 값) → 지금 연결의 클라이언트
func TestFileScopeOptions(t *testing.T) {
	current := map[string]string{"scope": "drive", "client_id": "old.apps.googleusercontent.com", "client_secret": "옛비밀"}
	got := FileScopeOptions(map[string]string{}, current)
	if got["scope"] != "drive.file" || got["client_id"] != current["client_id"] || got["client_secret"] != "옛비밀" {
		t.Fatalf("비워 두면 지금 클라이언트를 그대로 써야 함: %v", got)
	}
	got = FileScopeOptions(map[string]string{"client_id": " new.apps.googleusercontent.com ", "client_secret": "새비밀"}, current)
	if got["client_id"] != "new.apps.googleusercontent.com" || got["client_secret"] != "새비밀" {
		t.Fatalf("화면에서 넣은 값이 먼저: %v", got)
	}
}

func TestModeOfRemote(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "rclone.conf")
	text := "[gdrive]\ntype = drive\nscope = drive.file\nclient_id = 1-a.apps.googleusercontent.com\nclient_secret = 비밀\ntoken = {\"access_token\":\"x\"}\n\n[fake]\ntype = alias\nremote = /tmp\n"
	if err := os.WriteFile(conf, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	rc := &rclone.Client{Exe: "/없는/실행파일", Config: conf}
	if Of(rc, "gdrive") != GoogleDrive || Of(rc, "fake") != nil || Of(rc, "없음") != nil {
		t.Fatal("rclone type 으로 모드를 찾아야 함")
	}
	if !slices.Equal(SyncFlags(rc, "gdrive"), []string{"--drive-skip-gdocs"}) || SyncFlags(rc, "fake") != nil {
		t.Fatal("모드별 동기화 옵션")
	}
	conf2, _ := rc.RemoteOptions("gdrive")
	info := GoogleDrive.Info(conf2)
	if info["scope"] != "drive.file" || info["client_id"] != "1-a.apps.googleusercontent.com" || len(info) != 2 {
		t.Fatalf("화면에는 범위와 ID 만 보내야 함 (보안 비밀·토큰 제외): %v", info)
	}
}

func TestSFTPParams(t *testing.T) {
	got, err := SFTP.Params(map[string]string{"host": " nas.local ", "user": "me", "pass": "비밀"})
	if err != nil || !slices.Equal(got, []string{"host=nas.local", "port=22", "user=me", "pass=비밀"}) {
		t.Fatalf("비밀번호 방식, 포트 기본 22: %v %v", got, err)
	}
	got, err = SFTP.Params(map[string]string{"host": "10.0.0.2", "port": "2222", "user": "me", "auth": "key", "key_file": "~/.ssh/id_ed25519", "key_file_pass": "암호"})
	if err != nil || !slices.Equal(got, []string{"host=10.0.0.2", "port=2222", "user=me", "key_file=~/.ssh/id_ed25519", "key_file_pass=암호"}) {
		t.Fatalf("키 파일 방식: %v %v", got, err)
	}
	for _, in := range []map[string]string{
		{"user": "me", "pass": "x"},                                 // 호스트 없음
		{"host": "me@nas", "user": "me", "pass": "x"},               // 호스트에 @
		{"host": "nas", "port": "99999", "user": "me", "pass": "x"}, // 포트 범위
		{"host": "nas", "pass": "x"},                                // 사용자 없음
		{"host": "nas", "user": "me"},                               // 비밀번호 없음
		{"host": "nas", "user": "me", "auth": "key"},                // 키 파일 없음
	} {
		if _, err := SFTP.Params(in); err == nil {
			t.Fatalf("거절해야 함: %v", in)
		}
	}
	conf := map[string]string{"type": "sftp", "host": "nas", "user": "me", "pass": "가린값"}
	if info := SFTP.Info(conf); info["auth"] != "password" || info["port"] != "22" || info["pass"] != "" {
		t.Fatalf("화면 정보에 비밀번호 없음: %v", info)
	}
	if SFTP.Where(conf) != "me@nas:22" {
		t.Fatal(SFTP.Where(conf))
	}
}
