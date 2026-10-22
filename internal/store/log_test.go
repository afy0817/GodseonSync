package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 폴더 로그는 폴더마다 따로, Both 는 전체 로그에도 [이름] 을 붙여 남긴다. 지우기·정리·이상한 ID 거절.
func TestFolderLog(t *testing.T) {
	t.Setenv("GODSEONSYNC_HOME", t.TempDir())
	Log("GodseonSync 시작 (설정 화면 포트 %d)", 9417)
	doc, pic := FolderLog{ID: "f-doc", Name: "문서"}, FolderLog{ID: "f-pic", Name: "사진"}
	doc.Log("  ↑ 올림%s: %s", "", "a.txt")
	doc.Both("✓ 완료 (%s): %s", "1초", "올림 1개")
	pic.Log("  ↓ 받음%s: %s", "", "b.jpg")
	if l := LogTail("f-doc", 10); !strings.HasSuffix(l, " ✓ 완료 (1초): 올림 1개") || !strings.Contains(l, "a.txt") || strings.Contains(l, "b.jpg") {
		t.Fatalf("문서 로그:\n%s", l)
	}
	if l := LogTail("", 10); !strings.HasSuffix(l, " [문서] ✓ 완료 (1초): 올림 1개") || strings.Contains(l, "a.txt") || strings.Contains(l, "b.jpg") {
		t.Fatalf("전체 로그:\n%s", l)
	}
	if ids := strings.Join(FolderLogIDs(), ","); ids != "f-doc,f-pic" {
		t.Fatalf("로그가 있는 폴더: %s", ids)
	}

	// 이상한 ID 는 파일로 쓰지 않고 전체 로그에 남긴다
	for _, bad := range []string{"../x", "a/b", "", strings.Repeat("x", 65)} {
		if FolderLogFile(bad) != "" || LogTail(bad, 5) != "" && bad != "" {
			t.Fatalf("%q 는 폴더 로그로 쓰면 안 됨", bad)
		}
	}
	FolderLog{ID: "../x", Name: "이상함"}.Log("✗ 실패: %s", "x")
	if !strings.HasSuffix(LogTail("", 5), "[이상함] ✗ 실패: x") {
		t.Fatal("폴더 로그를 못 쓰면 전체 로그에")
	}
	if ClearFolderLog("../x", "테스트") == nil {
		t.Fatal("이상한 ID 는 거절")
	}

	// 돌려 둔 .1 의 끝부분을 이어 붙여 보여 준다
	f := FolderLogFile("f-pic")
	if err := os.Rename(f, f+".1"); err != nil {
		t.Fatal(err)
	}
	pic.Log("  ↓ 받음%s: %s", "", "c.jpg")
	if l := LogTail("f-pic", 10); !strings.Contains(l, "b.jpg") || !strings.HasSuffix(l, "c.jpg") {
		t.Fatalf(".1 이어 읽기:\n%s", l)
	}
	if l := LogTail("f-pic", 1); strings.Contains(l, "b.jpg") {
		t.Fatalf("n 줄만:\n%s", l)
	}

	if err := ClearFolderLog("f-doc", "테스트"); err != nil {
		t.Fatal(err)
	}
	if l := LogTail("f-doc", 10); strings.Contains(l, "a.txt") || !strings.Contains(l, "로그를 지웠습니다 (테스트)") || !strings.Contains(LogTail("", 10), "[문서] ✓ 완료") {
		t.Fatalf("폴더 로그만 지움:\n%s", l)
	}
	PruneFolderLogs(map[string]bool{"f-doc": true})
	if _, err := os.Stat(f + ".1"); !os.IsNotExist(err) || FolderLogIDs()[0] != "f-doc" || len(FolderLogIDs()) != 1 {
		t.Fatal("설정에서 빠진 폴더 로그는 .1 까지 지운다")
	}
	if err := ClearLog("테스트"); err != nil {
		t.Fatal(err)
	}
	if l := LogTail("", 10); strings.Contains(l, "시작") || len(FolderLogIDs()) != 0 {
		t.Fatalf("모두 지움: %s / %v", l, FolderLogIDs())
	}
	if _, err := os.Stat(filepath.Join(LogsDir())); err != nil {
		t.Fatal("logs 폴더는 남는다")
	}
}
