package rclone

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHasRemoteReadsConfigWithoutRclone(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "rclone.conf")
	if err := os.WriteFile(conf, []byte("[other]\ntype = local\n\n[gdrive]\ntype = drive\nscope = drive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Client{Exe: "/없는/실행파일", Config: conf} // rclone 을 실행하면 실패한다
	if !c.HasRemote("gdrive") || !c.HasRemote("other") {
		t.Fatal("설정 파일의 연결을 찾아야 함")
	}
	if c.HasRemote("drive") || c.HasRemote("gdriv") {
		t.Fatal("이름이 정확히 같을 때만 있다고 해야 함")
	}
	t.Setenv("RCLONE_CONFIG_FAKE_TYPE", "alias")
	if !c.HasRemote("fake") {
		t.Fatal("환경 변수로 만든 연결도 찾아야 함")
	}
	if (&Client{Config: filepath.Join(t.TempDir(), "없음.conf")}).HasRemote("gdrive") {
		t.Fatal("설정 파일이 없으면 없다고 해야 함")
	}
}

func TestRemoteOptionsAndReplace(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "rclone.conf")
	text := "[other]\ntype = local\n\n[gdrive]\ntype = drive\nscope = drive\nclient_id = \ntoken = {\"access_token\":\"old\"}\n\n[gdrive-new]\ntype = drive\nscope = drive\nclient_id = new.apps.googleusercontent.com\ntoken = {\"access_token\":\"new\"}\n"
	if err := os.WriteFile(conf, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Client{Exe: "/없는/실행파일", Config: conf}
	o, ok := c.RemoteOptions("gdrive")
	if !ok || o["type"] != "drive" || o["scope"] != "drive" || o["client_id"] != "" || o["token"] != `{"access_token":"old"}` {
		t.Fatalf("연결 항목을 읽어야 함: %v %v", o, ok)
	}
	if _, ok := c.RemoteOptions("없음"); ok {
		t.Fatal("없는 연결")
	}
	if err := c.ReplaceRemote("gdrive-new", "gdrive"); err != nil {
		t.Fatal(err)
	}
	o, _ = c.RemoteOptions("gdrive")
	if o["client_id"] != "new.apps.googleusercontent.com" || o["token"] != `{"access_token":"new"}` {
		t.Fatalf("새 연결이 원래 이름에 들어가야 함: %v", o)
	}
	if c.HasRemote("gdrive-new") || !c.HasRemote("other") {
		t.Fatal("임시 연결은 없어지고 다른 연결은 그대로여야 함")
	}
	if err := c.ReplaceRemote("gdrive-new", "gdrive"); err == nil {
		t.Fatal("없는 연결을 바꾸면 오류여야 함")
	}
	if st, _ := os.Stat(conf); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 { // 윈도우에는 유닉스 권한이 없다
		t.Fatalf("토큰이 든 파일이라 0600 이어야 함: %v", st.Mode().Perm())
	}
}
