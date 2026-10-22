package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/store"
)

// 실제 로그에서 가져온 rclone bisync 출력 (경로만 바꿈)
const bisyncChanged = `INFO  : Setting --ignore-listing-checksum as neither --checksum nor --compare checksum are set.
INFO  : Bisyncing with Comparison Settings:
{
	"Modtime": true,
	"Size": true,
	"Checksum": false,
	"HashType1": 0,
	"SlowHashDetected": true,
	"DownloadHash": false
}
INFO  : lock file renewed for 10m0s. New expiration: 2026-10-22 10:11:23.72645 +0900 KST m=+601.575677251
INFO  : Synching Path1 "local{6q8rt}:/Users/me/docs/" with Path2 "gdrive{YRXYK}:GodseonSync/docs/"
INFO  : Using filters file /Users/me/Library/Application Support/GodseonSync/filters/f-1.txt
INFO  : Building Path1 and Path2 listings
INFO  : Path1 checking for diffs
INFO  : - Path1             File changed: size (larger), time (newer)   - skills/a.md
INFO  : - Path1             File is new                                 - memory/new.md
INFO  : Path1:    2 changes:    1 new,    1 modified,    0 deleted
INFO  : (Modified:    1 newer,    0 older,    1 larger,    0 smaller)
INFO  : Path2 checking for diffs
INFO  : - Path2             File was deleted                            - old.txt
NOTICE: - WARNING           New or changed in both paths                - same.md
NOTICE: Local file system at /Users/me/docs: 0 differences found
NOTICE: Local file system at /Users/me/docs: 1 matching files
INFO  : Files are equal! Skipping: same.md
NOTICE: - WARNING           New or changed in both paths                - notes.md
INFO  : notes.md: Path1 is newer. Path1: 2026-10-22 09:59:22 +0900 KST, Path2: 2026-10-22 09:58:22 +0900 KST, Difference: 1m0s
INFO  : notes.md: The winner is: Path2
NOTICE: - Path1             Renaming Path1 copy                         - local{6q8rt}:/Users/me/docs/notes.conflict1.md
NOTICE: - Path1             Queue copy to Path2                         - gdrive{YRXYK}:GodseonSync/docs/notes.conflict1.md
INFO  : Applying changes
INFO  : - Path1             Queue copy to Path2                         - gdrive{YRXYK}:GodseonSync/docs/skills/a.md
INFO  : - Path1             Queue delete                                - local{6q8rt}:/Users/me/docs/old.txt
INFO  : - Path2             Do queued copies to                         - Path1
INFO  : old.txt: Deleted
INFO  : - Path1             Do queued copies to                         - Path2
INFO  : skills/a.md: Copied (replaced existing)
INFO  : memory/new.md: Copied (new)
INFO  : memory: Making directory
INFO  : memory: Made directory with metadata (mtime=2026-10-22T01:01:26.000000000Z)
INFO  : memory: Set directory modification time (using SetModTime)
INFO  :    10.000 KiB / 24.622 KiB, 41%, 24.619 KiB/s, ETA 1s
INFO  : Updating listings
INFO  : Validating listings for Path1 "local{6q8rt}:/Users/me/docs/" vs Path2 "gdrive{YRXYK}:GodseonSync/docs/"
INFO  : Bisync successful
INFO  :    24.622 KiB / 24.622 KiB, 100%, 24.619 KiB/s, ETA 0s
ERROR : Bisync critical error: something odd
ERROR : Bisync critical error: <nil>
NOTICE: Failed to bisync with 2 errors: last error was: bisync aborted
INFO  : Checking something new`

func TestReadableRcloneLog(t *testing.T) {
	t.Setenv("GODSEONSYNC_HOME", t.TempDir())
	lg := store.FolderLog{ID: "f-1", Name: "docs"}
	p := newRcloneLog(lg, "드라이브", false, "/Users/me/docs", "GodseonSync/docs")
	for _, l := range strings.Split(bisyncChanged, "\n") {
		p.line(l)
	}
	var got []string
	for _, l := range strings.Split(store.LogTail("f-1", 100), "\n") {
		got = append(got, l[len("2006-01-02 15:04:05 "):])
	}
	want := []string{
		"  ⚠ 양쪽에서 바뀜: notes.md — 더 최신 파일을 남기고, 다른 쪽은 이름 뒤에 번호를 붙여 남깁니다",
		"    이 PC 쪽 파일을 notes.conflict1.md 로 이름 바꿔 남김",
		"  − 이 PC 에서 지움: old.txt",
		"  ↑ 올림 (수정): skills/a.md",
		"  ↑ 올림 (새 파일): memory/new.md",
		"  ↑ 올림 (새 폴더): memory",
		"  … 진행 중: 10.000 KiB / 24.622 KiB (41%)",
		"  ✗ Bisync critical error: something odd",
		"  Checking something new",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("로그:\n%s\n\n기대:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if s := p.summary(); s != "올림 3개, 이 PC 에서 1개 지움, 충돌 1개, 24.622 KiB" {
		t.Fatalf("요약: %q", s)
	}
	if !strings.Contains(store.LogTail("", 10), "") || strings.Contains(store.LogTail("", 10), "올림") {
		t.Fatal("파일 줄은 전체 로그에 남기지 않는다")
	}
	if s := newRcloneLog(lg, "SFTP", false).summary(); s != "바뀐 파일 없음" {
		t.Fatalf("빈 요약: %q", s)
	}
}

// hasLine 은 로그에 parts 를 모두 담은 줄이 있는지다.
func hasLine(log string, parts ...string) bool {
	for _, l := range strings.Split(log, "\n") {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(l, p)
		}
		if ok {
			return true
		}
	}
	return false
}

// 실패 원인은 bisync 의 마무리 줄('Bisync aborted …')이 아니라 그 앞의 진짜 오류 줄에서 고른다.
func TestFailCause(t *testing.T) {
	tail := strings.Split(`INFO  : Building Path1 and Path2 listings
ERROR : failed to open directory "": open /a/사진: permission denied
ERROR : There were errors while building listings. Aborting as it is too dangerous to continue.
ERROR : Bisync critical error: <nil>
ERROR : Bisync aborted. Error is retryable without --resync due to --resilient mode.`, "\n")
	if c := failCause(tail); c != `failed to open directory "": open /a/사진: permission denied` {
		t.Fatalf("원인: %q", c)
	}
	if c := failCause(tail[3:]); c != "Bisync aborted. Error is retryable without --resync due to --resilient mode." {
		t.Fatalf("마무리 줄뿐이면 마지막 줄: %q", c)
	}
	if c := failCause([]string{"INFO  : 아무것도"}); c != "" {
		t.Fatalf("오류 없음: %q", c)
	}
}

// 폴더 로그는 폴더마다 따로 남고, 전체 로그에는 결과 한 줄만 [폴더 이름] 과 함께 남는다.
// 바뀐 게 없을 때 rclone 이 매번 내는 줄은 어디에도 남지 않고, 설정에서 빠진 폴더의 로그는 지운다.
func TestFolderLogs(t *testing.T) {
	a, b, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(a.local, "a.txt"), "A", t0)
	write(t, filepath.Join(a.local, "sub", "b.txt"), "B", t0)
	other := filepath.Join(filepath.Dir(a.local), "other")
	write(t, filepath.Join(other, "o.txt"), "O", t0)
	a.use(t)
	saveConfig(t,
		config.Folder{ID: "f-d", Name: "문서", DrivePath: "GodseonSync/d", Paths: map[string]string{a.id: a.local, b.id: b.local}},
		config.Folder{ID: "f-o", Name: "기타", DrivePath: "GodseonSync/o", Paths: map[string]string{a.id: other}})
	a.sync(t, Options{})
	b.sync(t, Options{})

	a.use(t)
	if l := store.LogTail("f-d", 100); !strings.Contains(l, "▶ 동기화 시작 (합치기): "+a.local) ||
		!hasLine(l, "↑ 올림", ": a.txt") || !hasLine(l, "↑ 올림", ": sub/b.txt") ||
		!strings.Contains(l, "✓ 합치기 완료") || !strings.Contains(l, "올림 3개") || strings.Contains(l, "o.txt") {
		t.Fatalf("A 의 문서 폴더 로그:\n%s", l)
	}
	if l := store.LogTail("f-o", 100); !hasLine(l, "↑ 올림", ": o.txt") || strings.Contains(l, "a.txt") {
		t.Fatalf("A 의 기타 폴더 로그:\n%s", l)
	}
	b.use(t)
	if l := store.LogTail("f-d", 100); !hasLine(l, "↓ 받음", ": a.txt") || !strings.Contains(l, "받음 3개") {
		t.Fatalf("B 의 문서 폴더 로그:\n%s", l)
	}

	// 지우면 반대쪽에서도 지운다 / 양쪽에서 고치면 충돌
	t1 := t0.Add(10 * time.Minute)
	a.use(t)
	if err := os.Remove(filepath.Join(a.local, "a.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(a.local, "sub", "b.txt"), "A 가 고침", t1)
	a.sync(t, Options{})
	write(t, filepath.Join(b.local, "sub", "b.txt"), "B 가 더 나중에 고침", t1.Add(time.Minute))
	b.sync(t, Options{})
	a.use(t)
	if l := store.LogTail("f-d", 100); !strings.Contains(l, "− 드라이브에서 지움: a.txt") || !strings.Contains(l, "↑ 올림 (수정): sub/b.txt") {
		t.Fatalf("A 의 지움·수정:\n%s", l)
	}
	b.use(t)
	l := store.LogTail("f-d", 100)
	if !strings.Contains(l, "− 이 PC 에서 지움: a.txt") || !strings.Contains(l, "⚠ 양쪽에서 바뀜: sub/b.txt") || !strings.Contains(l, "충돌 1개") {
		t.Fatalf("B 의 지움·충돌:\n%s", l)
	}
	all := store.LogTail("", 200)
	for _, noise := range []string{"Comparison Settings", "lock file renewed", "Path1 checking", "Bisync successful", "\"Modtime\"", "Queue copy"} {
		if strings.Contains(all+l, noise) {
			t.Fatalf("남기지 않아야 하는 줄 %q:\n%s\n---\n%s", noise, all, l)
		}
	}
	if !strings.Contains(all, "[문서] ✓ 완료 (") || strings.Contains(all, "sub/b.txt") {
		t.Fatalf("B 의 전체 로그:\n%s", all)
	}
	if exists(store.FolderLogFile("f-o")) {
		t.Fatal("B 는 기타 폴더를 동기화하지 않으니 로그가 없어야 함")
	}

	// 바뀐 게 없으면 시작·끝 두 줄
	before := strings.Count(store.LogTail("f-d", 1000), "\n")
	b.sync(t, Options{})
	if l := store.LogTail("f-d", 1000); strings.Count(l, "\n")-before != 2 || !strings.HasSuffix(l, "바뀐 파일 없음") {
		t.Fatalf("변경 없음:\n%s", l)
	}

	// 설정에서 빠진 폴더의 로그는 지운다
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-d", Name: "문서", DrivePath: "GodseonSync/d", Paths: map[string]string{a.id: a.local, b.id: b.local}})
	a.sync(t, Options{})
	if exists(store.FolderLogFile("f-o")) || !exists(store.FolderLogFile("f-d")) {
		t.Fatal("빠진 폴더의 로그만 지워야 함")
	}
	_ = drive
}
