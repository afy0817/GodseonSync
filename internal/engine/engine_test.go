package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/rcloneembed"
	"github.com/afy0817/GodseonSync/internal/store"
)

// 테스트 바이너리도 `<바이너리> rclone ...` 으로 불리면 rclone 처럼 동작하게 한다.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "rclone" {
		rcloneembed.Main(os.Args[2:])
	}
	config.NotFoundRetry.Delay = 0 // 가짜 드라이브(로컬 폴더)는 색인 지연이 없다
	os.Exit(m.Run())
}

type pc struct {
	id, home, local string
}

// use 는 이 PC 의 데이터 폴더로 바꾼다 (테스트 안에서 PC 두 대를 번갈아 흉내 낸다).
func (p pc) use(t *testing.T) {
	t.Helper()
	t.Setenv("GODSEONSYNC_HOME", p.home)
}

func (p pc) sync(t *testing.T, opt Options) Result {
	t.Helper()
	p.use(t)
	res, err := Run(opt)
	if err != nil {
		t.Fatalf("%s 동기화 오류: %v\n%s", p.id, err, store.LogTail(60))
	}
	if len(res.Failed) > 0 {
		t.Fatalf("%s 동기화 실패: %v\n%s", p.id, res.Failed, store.LogTail(60))
	}
	return res
}

func write(t *testing.T, path, text string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("읽기 실패 %s: %v", path, err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func setup(t *testing.T) (a, b pc, drive string) {
	t.Helper()
	base, err := os.MkdirTemp("", "gs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	base, _ = filepath.EvalSymlinks(base)
	drive = filepath.Join(base, "drive")
	if err := os.MkdirAll(drive, 0o755); err != nil {
		t.Fatal(err)
	}
	// 가짜 구글 드라이브: rclone alias 연결 "fake" 가 로컬 폴더를 가리킨다
	t.Setenv("RCLONE_CONFIG_FAKE_TYPE", "alias")
	t.Setenv("RCLONE_CONFIG_FAKE_REMOTE", drive)

	a = pc{id: "pc-a", home: filepath.Join(base, "home-a"), local: filepath.Join(base, "a", "docs")}
	b = pc{id: "pc-b", home: filepath.Join(base, "home-b"), local: filepath.Join(base, "b", "docs")}
	for _, p := range []pc{a, b} {
		p.use(t)
		if err := os.MkdirAll(filepath.Dir(p.local), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveSettings(store.Settings{ClientID: p.id, ClientName: strings.ToUpper(p.id),
			Remote: "fake", Root: "GodseonSync", IntervalMinutes: 10, Configured: true}); err != nil {
			t.Fatal(err)
		}
	}
	return a, b, drive
}

// saveConfig 는 드라이브의 setting.json 에 폴더 목록을 넣는다 (없으면 GodseonSync 폴더와 함께 만든다).
func saveConfig(t *testing.T, folders ...config.Folder) {
	t.Helper()
	d := NewDrive(store.LoadSettings())
	if _, _, err := d.Ensure("test", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Update("test", func(c *config.Config) error {
		c.Folders = folders
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTwoPCsSync(t *testing.T) {
	a, b, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)

	// PC A 에 파일·하위 폴더·빈 폴더·제외 대상·한글(NFD) 이름 파일을 만든다
	write(t, filepath.Join(a.local, "a.txt"), "A1", t0)
	write(t, filepath.Join(a.local, "sub", "deep", "x.txt"), "X", t0)
	write(t, filepath.Join(a.local, ".DS_Store"), "junk", t0)
	write(t, filepath.Join(a.local, norm.NFD.String("한글파일.txt")), "KO", t0)
	if err := os.MkdirAll(filepath.Join(a.local, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}

	a.use(t)
	saveConfig(t, config.Folder{Name: "docs", DrivePath: "GodseonSync/docs",
		Paths: map[string]string{a.id: a.local, b.id: b.local}})

	a.sync(t, Options{})
	dd := filepath.Join(drive, "GodseonSync", "docs")
	if read(t, filepath.Join(dd, "sub", "deep", "x.txt")) != "X" {
		t.Fatal("하위 폴더 구조가 드라이브에 올라가지 않음")
	}
	if exists(filepath.Join(dd, ".DS_Store")) {
		t.Fatal(".DS_Store 는 제외돼야 함")
	}
	if !exists(filepath.Join(dd, "emptydir")) {
		t.Fatal("빈 폴더도 올라가야 함")
	}
	if runtime.GOOS == "darwin" && !exists(filepath.Join(dd, norm.NFC.String("한글파일.txt"))) {
		t.Fatal("맥에서는 한글 파일 이름이 드라이브에 NFC 로 있어야 함")
	}

	// PC B: 로컬 폴더가 없으면 만들고 내려받는다
	b.sync(t, Options{})
	if read(t, filepath.Join(b.local, "a.txt")) != "A1" || read(t, filepath.Join(b.local, "sub", "deep", "x.txt")) != "X" {
		t.Fatal("PC B 로 내려받지 못함")
	}
	if !exists(filepath.Join(b.local, "emptydir")) {
		t.Fatal("PC B 에 빈 폴더가 없음")
	}

	// PC B 에서 수정·삭제·추가 → A 로 전달
	write(t, filepath.Join(b.local, "a.txt"), "A2", t0.Add(10*time.Minute))
	if err := os.Remove(filepath.Join(b.local, "sub", "deep", "x.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(b.local, "new", "b.txt"), "B", t0.Add(10*time.Minute))
	b.sync(t, Options{})
	a.sync(t, Options{})
	if read(t, filepath.Join(a.local, "a.txt")) != "A2" {
		t.Fatal("수정이 A 로 오지 않음")
	}
	if exists(filepath.Join(a.local, "sub", "deep", "x.txt")) {
		t.Fatal("삭제가 A 로 오지 않음")
	}
	if read(t, filepath.Join(a.local, "new", "b.txt")) != "B" {
		t.Fatal("추가가 A 로 오지 않음")
	}

	// 양쪽에서 같은 파일을 고치면 최신이 이기고 진 쪽은 conflict 이름으로 남는다
	write(t, filepath.Join(a.local, "c.txt"), "C0", t0)
	a.sync(t, Options{})
	b.sync(t, Options{})
	write(t, filepath.Join(a.local, "c.txt"), "C-from-A", t0.Add(20*time.Minute))
	write(t, filepath.Join(b.local, "c.txt"), "C-from-B", t0.Add(30*time.Minute))
	a.sync(t, Options{})
	b.sync(t, Options{})
	a.sync(t, Options{})
	if read(t, filepath.Join(a.local, "c.txt")) != "C-from-B" || read(t, filepath.Join(b.local, "c.txt")) != "C-from-B" {
		t.Fatal("더 최신(B) 파일이 이겨야 함")
	}
	entries, _ := os.ReadDir(a.local)
	conflict := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "conflict") {
			conflict = true
		}
	}
	if !conflict {
		t.Fatal("진 파일이 conflict 이름으로 남아야 함")
	}

	// 두 PC 가 setting.json 의 PC 목록에 올라가고, 각자 .status 에 상태를 올렸다
	d := NewDrive(store.LoadSettings())
	cfg, err := d.Load()
	if err != nil {
		t.Fatal(err)
	}
	if pa, pb := cfg.FindPC(a.id), cfg.FindPC(b.id); pa == nil || pb == nil || pa.Name != "PC-A" || pb.Name != "PC-B" {
		t.Fatalf("PC 목록이 setting.json 에 없음: %+v", cfg.PCs)
	}
	statuses, err := d.LoadStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if statuses[a.id].LastSeen == "" || len(statuses[b.id].Folders) == 0 {
		t.Fatalf("PC 상태가 .status 에 없음: %+v", statuses)
	}
	if !exists(filepath.Join(drive, "GodseonSync", "setting.json")) || !exists(filepath.Join(drive, "GodseonSync", ".status", b.id+".json")) {
		t.Fatal("드라이브에 GodseonSync/setting.json 과 .status 가 있어야 함")
	}
}

func TestPathChangeTriggersResync(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(a.local, "one.txt"), "1", t0)
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-x", Name: "x", DrivePath: "GodseonSync/x", Paths: map[string]string{a.id: a.local}})
	a.sync(t, Options{})
	if !exists(filepath.Join(drive, "GodseonSync", "x", "one.txt")) {
		t.Fatal("파일이 올라가지 않음")
	}
	// 로컬 경로를 바꾸면 기록이 달라져 합치기를 해야 한다. 양쪽에 파일이 있으니 먼저 묻는다
	other := filepath.Join(filepath.Dir(a.local), "docs2")
	write(t, filepath.Join(other, "two.txt"), "2", t0)
	saveConfig(t, config.Folder{ID: "f-x", Name: "x", DrivePath: "GodseonSync/x", Paths: map[string]string{a.id: other}})
	if res := a.sync(t, Options{}); len(res.NeedChoice) != 1 || exists(filepath.Join(other, "one.txt")) {
		t.Fatalf("양쪽에 파일이 있으면 멈추고 물어야 함: %+v", res)
	}
	// 합치기를 고르면 지우지 않고 합친다
	if err := store.SetChoice("f-x", store.ChoiceMerge); err != nil {
		t.Fatal(err)
	}
	a.sync(t, Options{})
	if read(t, filepath.Join(other, "one.txt")) != "1" || !exists(filepath.Join(drive, "GodseonSync", "x", "two.txt")) {
		t.Fatal("경로를 바꾼 뒤 합치기가 되지 않음")
	}
	if store.Choice("f-x") != "" || store.LoadStatus().Folders["f-x"].NeedsChoice {
		t.Fatal("끝나면 고른 값과 결정 필요 표시가 지워져야 함")
	}
}

func TestRemovedPCRegistersAgain(t *testing.T) {
	a, b, _ := setup(t)
	a.use(t)
	saveConfig(t)
	b.sync(t, Options{})
	a.use(t)
	d := NewDrive(store.LoadSettings())
	if _, err := d.Update("A", func(c *config.Config) error {
		c.PCs = nil // PC A 에서 B 를 목록에서 뺐다
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	b.sync(t, Options{})
	cfg, err := d.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FindPC(b.id) == nil {
		t.Fatal("B 에서 GodseonSync 가 돌면 PC 목록에 다시 올라가야 함")
	}
}

func TestLegacyLayoutMigrates(t *testing.T) {
	a, _, drive := setup(t)
	legacy := filepath.Join(drive, "GodseonSync", ".godseonsync")
	write(t, filepath.Join(legacy, "config.json"), `{"app":"GodseonSync","version":1,"revision":5,"exclude":[".DS_Store"],
		"folders":[{"id":"f-old","name":"old","drive_path":"GodseonSync/old","paths":{"pc-a":"/Users/x/old"}}]}`, time.Now())
	write(t, filepath.Join(legacy, "clients", "pc-a.json"), `{"id":"pc-a","name":"예전 맥","os":"mac"}`, time.Now())
	a.use(t)
	cfg, created, err := NewDrive(store.LoadSettings()).Ensure("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !created || len(cfg.Folders) != 1 || cfg.Folders[0].Paths["pc-a"] != "/Users/x/old" {
		t.Fatalf("예전 설정을 옮겨야 함: %+v", cfg)
	}
	if pc := cfg.FindPC("pc-a"); pc == nil || pc.Name != "예전 맥" {
		t.Fatalf("예전 PC 목록을 옮겨야 함: %+v", cfg.PCs)
	}
	if !exists(filepath.Join(drive, "GodseonSync", "setting.json")) {
		t.Fatal("setting.json 이 만들어져야 함")
	}
}

func TestOnlyFoldersWithThisPCPath(t *testing.T) {
	a, b, _ := setup(t)
	a.use(t)
	saveConfig(t, config.Folder{Name: "onlyB", DrivePath: "GodseonSync/onlyB", Paths: map[string]string{b.id: b.local}})
	res := a.sync(t, Options{})
	if res.Synced != 0 {
		t.Fatalf("PC A 경로가 없는 폴더는 건너뛰어야 함 (synced=%d)", res.Synced)
	}
}

func TestLongLocalPathUsesShortLink(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	long := filepath.Dir(a.local)
	for i := 0; i < 6; i++ {
		long = filepath.Join(long, "아주긴한글폴더이름입니다") // 한글은 UTF-8 로 3바이트
	}
	write(t, filepath.Join(long, "deep.txt"), "D", t0)
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-long", Name: "long", DrivePath: "GodseonSync/long", Paths: map[string]string{a.id: long}})
	if config.SessionNameBytes(long, "fake:GodseonSync/long") <= 255 {
		t.Fatal("테스트 경로가 충분히 길지 않음")
	}
	a.sync(t, Options{})
	if read(t, filepath.Join(drive, "GodseonSync", "long", "deep.txt")) != "D" {
		t.Fatal("긴 경로 폴더가 올라가지 않음")
	}
	write(t, filepath.Join(drive, "GodseonSync", "long", "back.txt"), "B", t0)
	a.sync(t, Options{})
	if read(t, filepath.Join(long, "back.txt")) != "B" {
		t.Fatal("긴 경로 폴더로 내려받지 못함")
	}
}

// 예전 개발판으로 처음 설정을 끝낸 PC 는 새 판으로 바꾼 뒤 첫 동기화 때 setting.json 으로 옮겨지고 PC ID 를 그대로 쓴다.
func TestConfiguredPCMigratesLegacyOnSync(t *testing.T) {
	a, _, drive := setup(t)
	legacy := filepath.Join(drive, "GodseonSync", ".godseonsync")
	write(t, filepath.Join(legacy, "config.json"), `{"app":"GodseonSync","version":1,"revision":1,"exclude":[".DS_Store"],"folders":[]}`, time.Now())
	write(t, filepath.Join(legacy, "clients", a.id+".json"), `{"id":"`+a.id+`","name":"PC-A","os":"mac"}`, time.Now())
	a.sync(t, Options{})
	cfg, err := NewDrive(store.LoadSettings()).Load()
	if err != nil {
		t.Fatalf("setting.json 이 만들어져야 함: %v", err)
	}
	if len(cfg.PCs) != 1 || cfg.PCs[0].ID != a.id {
		t.Fatalf("예전 PC ID 를 그대로 써야 함: %+v", cfg.PCs)
	}
}

// 드라이브에서 setting.json 이 지워지면 이 PC 가 마지막으로 받은 설정으로 다시 만들고 동기화를 이어간다.
func TestMissingSettingRecreatedFromCache(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(a.local, "keep.txt"), "K", t0)
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-k", Name: "k", DrivePath: "GodseonSync/k", Paths: map[string]string{a.id: a.local}})
	a.sync(t, Options{})
	if err := os.Remove(filepath.Join(drive, "GodseonSync", "setting.json")); err != nil {
		t.Fatal(err)
	}
	a.sync(t, Options{})
	cfg, err := NewDrive(store.LoadSettings()).Load()
	if err != nil {
		t.Fatalf("setting.json 이 다시 만들어져야 함: %v", err)
	}
	if len(cfg.Folders) != 1 || cfg.Folders[0].Paths[a.id] != a.local || cfg.FindPC(a.id) == nil {
		t.Fatalf("마지막으로 받은 설정(폴더·PC)으로 만들어야 함: %+v", cfg)
	}
}

func TestCreateWritesGivenSetting(t *testing.T) {
	a, _, drive := setup(t)
	a.use(t)
	d := NewDrive(store.LoadSettings())
	c := &config.Config{PCs: []config.PC{{ID: a.id, Name: "PC-A", OS: platform.OSKey}}, // 경로 검사는 PC 의 OS 기준이라 지금 OS 로
		Folders: []config.Folder{{Name: "새 폴더", DrivePath: "GodseonSync/새 폴더", Paths: map[string]string{a.id: a.local}}}}
	saved, err := d.Create(c, "test")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 || saved.Folders[0].ID == "" || !exists(filepath.Join(drive, "GodseonSync", "setting.json")) {
		t.Fatalf("저장하려는 설정으로 setting.json 을 만들어야 함: %+v", saved)
	}
	if _, err := d.Create(c, "test"); err == nil {
		t.Fatal("이미 있으면 덮어쓰지 않아야 함")
	}
}

// 설정 창과 예약 동기화가 거의 동시에 Ensure 를 불러도 한 번만 옮기고 둘 다 성공해야 한다.
func TestConcurrentEnsureMigratesOnce(t *testing.T) {
	a, _, drive := setup(t)
	legacy := filepath.Join(drive, "GodseonSync", ".godseonsync")
	write(t, filepath.Join(legacy, "config.json"), `{"app":"GodseonSync","version":1,"revision":1,"folders":[]}`, time.Now())
	a.use(t)
	d := NewDrive(store.LoadSettings())
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			_, _, err := d.Ensure("test", nil)
			errs <- err
		}()
	}
	for i := 0; i < 4; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("동시에 불러도 실패하면 안 됨: %v", err)
		}
	}
	if n := strings.Count(store.LogTail(100), "예전 형식 설정"); n != 1 {
		t.Fatalf("한 번만 옮겨야 함 (로그 %d 번)", n)
	}
}

// 동기화 중에 GodseonSync 를 끄면 rclone 을 멈추고, 다음 동기화가 이어서 끝낸다.
func TestInterruptThenResume(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	for i := 0; i < 3000; i++ {
		write(t, filepath.Join(a.local, fmt.Sprintf("d%02d", i%30), fmt.Sprintf("f%04d.txt", i)), "x", t0)
	}
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-big", Name: "big", DrivePath: "GodseonSync/big", Paths: map[string]string{a.id: a.local}})
	done := make(chan Result, 1)
	go func() {
		res, _ := Run(Options{})
		done <- res
	}()
	for i := 0; i < 100 && store.ReadLock() == nil; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(150 * time.Millisecond)
	Interrupt()
	res := <-done
	if len(res.Failed) == 0 {
		t.Skip("멈추기 전에 동기화가 끝나 버림 (이 PC 가 너무 빠름)")
	}
	if st := store.LoadStatus().Folders["f-big"]; st == nil || !strings.Contains(st.LastError, "중간에 멈춤") {
		t.Fatalf("멈춘 기록이 있어야 함: %+v", st)
	}
	a.sync(t, Options{})
	files, _ := filepath.Glob(filepath.Join(drive, "GodseonSync", "big", "*", "*.txt"))
	if len(files) != 3000 {
		t.Fatalf("다음 동기화에서 모두 올라가야 함 (%d/3000)", len(files))
	}
	partials, _ := filepath.Glob(filepath.Join(drive, "GodseonSync", "big", "*", "*.partial"))
	local, _ := filepath.Glob(filepath.Join(a.local, "*", "*.txt"))
	if len(partials) != 0 || len(local) != 3000 {
		t.Fatalf("임시 파일만 지우고 진짜 파일은 그대로여야 함 (partial %d, 로컬 %d)", len(partials), len(local))
	}
}

func TestPartialFilesNeverSync(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(a.local, "real.txt"), "R", t0)
	write(t, filepath.Join(a.local, "real.txt.0a1b2c3d.partial"), "half", t0)
	write(t, filepath.Join(a.local, "my.partial"), "user file", t0) // 사용자가 만든 비슷한 이름은 동기화한다
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-p", Name: "p", DrivePath: "GodseonSync/p", Paths: map[string]string{a.id: a.local}})
	a.sync(t, Options{})
	dd := filepath.Join(drive, "GodseonSync", "p")
	if !exists(filepath.Join(dd, "real.txt")) || !exists(filepath.Join(dd, "my.partial")) || exists(filepath.Join(dd, "real.txt.0a1b2c3d.partial")) {
		t.Fatal("rclone 임시 파일만 빼고 동기화해야 함")
	}
}

// 저장소 세대가 비어 있으면 예전과 같은 값(업데이트만으로 다시 합치지 않음), 바뀌면 다른 값(모든 폴더 합치기).
func TestFolderSigEpoch(t *testing.T) {
	sum := sha256.Sum256([]byte("t\nl\nr\nf"))
	if got := folderSig("t", "l", "r", "f", ""); got != hex.EncodeToString(sum[:]) {
		t.Fatal("세대가 비면 세대를 넣기 전과 같아야 함")
	}
	if folderSig("t", "l", "r", "f", "e-1") == folderSig("t", "l", "r", "f", "") || folderSig("t", "l", "r", "f", "e-1") == folderSig("t", "l", "r", "f", "e-2") {
		t.Fatal("세대가 바뀌면 달라야 함")
	}
}

// backupDirs 는 dir 옆에 생긴 `<이름>_backup_<14자리>` 폴더들이다.
func backupDirs(t *testing.T, dir string) []string {
	t.Helper()
	all, _ := filepath.Glob(dir + "_backup_*")
	var out []string
	for _, p := range all {
		if len(strings.TrimPrefix(p, dir+"_backup_")) == 14 {
			out = append(out, p)
		}
	}
	return out
}

// 양쪽에 파일이 있으면 멈추고, 고른 쪽으로 덮어쓰기 전에 덮어쓸 쪽을 _backup_년월일시분초 로 복사해 둔다.
func TestChoiceOverwritesWithBackup(t *testing.T) {
	a, b, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	dd := filepath.Join(drive, "GodseonSync", "docs")
	write(t, filepath.Join(a.local, "a1.txt"), "A", t0)
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-d", Name: "docs", DrivePath: "GodseonSync/docs", Paths: map[string]string{a.id: a.local, b.id: b.local}})
	a.sync(t, Options{}) // 드라이브가 비어 있어 묻지 않고 올린다
	if !exists(filepath.Join(dd, "a1.txt")) || len(backupDirs(t, a.local)) != 0 {
		t.Fatal("한쪽이 비면 묻지도, 백업하지도 않고 올려야 함")
	}

	// PC B: 이 PC 에도 파일이 있다 → 멈추고 아무것도 바꾸지 않는다 (미리보기도 마찬가지)
	write(t, filepath.Join(b.local, "b1.txt"), "B", t0)
	write(t, filepath.Join(b.local, "sub", "b2.txt"), "B2", t0)
	if res := b.sync(t, Options{DryRun: true}); len(res.NeedChoice) != 0 || store.LoadStatus().Folders["f-d"] != nil && store.LoadStatus().Folders["f-d"].NeedsChoice {
		t.Fatal("미리보기는 결정 필요 기록을 남기지 않아야 함")
	}
	res := b.sync(t, Options{})
	fst := store.LoadStatus().Folders["f-d"]
	if len(res.NeedChoice) != 1 || !fst.NeedsChoice || fst.LocalFiles != 2 || fst.DriveFiles != 1 {
		t.Fatalf("양쪽에 파일이 있으면 멈추고 파일 수를 남겨야 함: %+v %+v", res, fst)
	}
	if exists(filepath.Join(b.local, "a1.txt")) || exists(filepath.Join(dd, "b1.txt")) {
		t.Fatal("고르기 전에는 아무것도 바꾸면 안 됨")
	}

	// 이 PC → 드라이브: 드라이브 폴더를 백업한 뒤 이 PC 내용으로 맞춘다
	if err := store.SetChoice("f-d", store.ChoiceLocal); err != nil {
		t.Fatal(err)
	}
	b.sync(t, Options{})
	bk := backupDirs(t, dd)
	if len(bk) != 1 || read(t, filepath.Join(bk[0], "a1.txt")) != "A" {
		t.Fatalf("드라이브 폴더 백업이 있어야 함: %v", bk)
	}
	if exists(filepath.Join(dd, "a1.txt")) || read(t, filepath.Join(dd, "sub", "b2.txt")) != "B2" || exists(filepath.Join(b.local, "a1.txt")) {
		t.Fatal("드라이브가 이 PC 와 같아져야 함")
	}
	if store.Choice("f-d") != "" || store.LoadStatus().Folders["f-d"].NeedsChoice || !store.LoadStatus().Folders["f-d"].Initialized {
		t.Fatal("끝나면 고른 값·결정 필요 표시가 지워지고 기록이 생겨야 함")
	}

	// PC A 를 파일이 있는 다른 폴더로 바꾼다 → 다시 묻고, 드라이브 → 이 PC 를 고르면 이 PC 폴더를 백업한 뒤 덮어쓴다
	other := filepath.Join(filepath.Dir(a.local), "docs2")
	write(t, filepath.Join(other, "mine.txt"), "M", t0)
	write(t, filepath.Join(other, ".DS_Store"), "junk", t0) // 제외 대상은 덮어쓰기에서도 건드리지 않는다
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-d", Name: "docs", DrivePath: "GodseonSync/docs", Paths: map[string]string{a.id: other, b.id: b.local}})
	if res := a.sync(t, Options{}); len(res.NeedChoice) != 1 {
		t.Fatalf("다시 물어야 함: %+v", res)
	}
	if err := store.SetChoice("f-d", store.ChoiceDrive); err != nil {
		t.Fatal(err)
	}
	a.sync(t, Options{})
	lb := backupDirs(t, other)
	if len(lb) != 1 || read(t, filepath.Join(lb[0], "mine.txt")) != "M" || read(t, filepath.Join(lb[0], ".DS_Store")) != "junk" {
		t.Fatalf("이 PC 폴더 백업이 (제외 대상까지) 통째로 있어야 함: %v", lb)
	}
	if exists(filepath.Join(other, "mine.txt")) || read(t, filepath.Join(other, "b1.txt")) != "B" || read(t, filepath.Join(other, "sub", "b2.txt")) != "B2" {
		t.Fatal("이 PC 가 드라이브와 같아져야 함")
	}
	if read(t, filepath.Join(other, ".DS_Store")) != "junk" {
		t.Fatal("제외 대상 파일은 덮어쓰기에서 지우면 안 됨")
	}
}

// '합치기' 버튼(Resync)은 합치기를 고른 것이라 묻지 않는다.
func TestManualResyncDoesNotAsk(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(drive, "GodseonSync", "m", "d.txt"), "D", t0)
	write(t, filepath.Join(a.local, "l.txt"), "L", t0)
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-m", Name: "m", DrivePath: "GodseonSync/m", Paths: map[string]string{a.id: a.local}})
	if res := a.sync(t, Options{Resync: true}); len(res.NeedChoice) != 0 {
		t.Fatalf("합치기 버튼은 묻지 않아야 함: %+v", res)
	}
	if !exists(filepath.Join(a.local, "d.txt")) || !exists(filepath.Join(drive, "GodseonSync", "m", "l.txt")) || len(backupDirs(t, a.local)) != 0 {
		t.Fatal("합치기는 양쪽을 남기고 백업하지 않음")
	}
}

// 합치기가 필요해도 양쪽 파일이 같으면(경로·크기·수정 시각) 묻지 않고 합친다. 맥의 자모 분리(NFD) 이름도 같은 이름으로 본다.
func TestIdenticalBothSidesMergeWithoutAsking(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(a.local, "a.txt"), "A", t0)
	write(t, filepath.Join(a.local, "sub", norm.NFD.String("한글.txt")), "K", t0)
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-s", Name: "s", DrivePath: "GodseonSync/s", Paths: map[string]string{a.id: a.local}})
	a.sync(t, Options{})
	// 같은 내용·같은 시각의 복사본으로 경로를 바꾼다 → 합치기가 필요하지만 양쪽이 같다
	other := filepath.Join(filepath.Dir(a.local), "docs-copy")
	write(t, filepath.Join(other, "a.txt"), "A", t0)
	write(t, filepath.Join(other, "sub", norm.NFD.String("한글.txt")), "K", t0)
	saveConfig(t, config.Folder{ID: "f-s", Name: "s", DrivePath: "GodseonSync/s", Paths: map[string]string{a.id: other}})
	if res := a.sync(t, Options{}); len(res.NeedChoice) != 0 {
		t.Fatalf("양쪽이 같으면 묻지 않아야 함: %+v\n%s", res, store.LogTail(20))
	}
	fst := store.LoadStatus().Folders["f-s"]
	if fst.NeedsChoice || !fst.Initialized || !strings.Contains(store.LogTail(40), "같아") {
		t.Fatalf("같아서 합쳤다는 기록: %+v", fst)
	}
	if store.Choice("f-s") != "" || exists(filepath.Join(drive, "GodseonSync", "s_backup")) {
		t.Fatal("고른 값·백업이 남으면 안 됨")
	}
}

// 다르면 묻고, 이 PC 에만 / 드라이브에만 / 내용이 다른 파일 수와 예시를 남긴다.
func TestChoiceDiffCounts(t *testing.T) {
	a, _, drive := setup(t)
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	dd := filepath.Join(drive, "GodseonSync", "c")
	write(t, filepath.Join(dd, "same.txt"), "S", t0)
	write(t, filepath.Join(dd, "drive-only.txt"), "D", t0)
	write(t, filepath.Join(dd, "changed.txt"), "드라이브 쪽", t0)
	write(t, filepath.Join(a.local, "same.txt"), "S", t0)
	write(t, filepath.Join(a.local, "pc-only-1.txt"), "P", t0)
	write(t, filepath.Join(a.local, "pc-only-2.txt"), "P", t0)
	write(t, filepath.Join(a.local, "changed.txt"), "이 PC 쪽에서 고침", t0.Add(30*time.Minute))
	a.use(t)
	saveConfig(t, config.Folder{ID: "f-c", Name: "c", DrivePath: "GodseonSync/c", Paths: map[string]string{a.id: a.local}})
	if res := a.sync(t, Options{}); len(res.NeedChoice) != 1 {
		t.Fatalf("다르면 물어야 함: %+v", res)
	}
	d := store.LoadStatus().Folders["f-c"].Diff
	if d == nil || d.OnlyLocal != 2 || d.OnlyDrive != 1 || d.Different != 1 ||
		strings.Join(d.OnlyLocalSamples, ",") != "pc-only-1.txt,pc-only-2.txt" || d.OnlyDriveSamples[0] != "drive-only.txt" || d.DifferentSamples[0] != "changed.txt" {
		t.Fatalf("차이: %+v", d)
	}
}
