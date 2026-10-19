// Package engine 은 동기화를 실행한다. 폴더마다 `rclone bisync <이 PC 경로> <드라이브 경로>` 를 돌린다.
//
//   - 처음 동기화하거나 경로·제외 규칙·저장소 세대가 바뀌면 합치기(--resync)로 돈다. 합치기는 아무것도 지우지 않고,
//     양쪽에 같은 파일이 있으면 더 최신 파일을 남긴다.
//   - 단, 합치기 전에 이 PC 와 드라이브 양쪽에 파일이 있으면 그 폴더는 멈추고 사용자가 고르게 한다 (store.SetChoice):
//     드라이브 → 이 PC 덮어쓰기, 이 PC → 드라이브 덮어쓰기, 합치기. 덮어쓰기 전에는 덮어쓸 쪽을
//     옆에 `<이름>_backup_<년월일시분초>` 로 복사해 둔다 (overwrite).
//   - 그 뒤로는 바뀐 쪽을 반대쪽에 반영한다 (추가·수정·삭제, 폴더 구조 포함). 양쪽에서 같은 파일을 고쳤으면
//     최신 파일이 이기고, 진 파일은 `이름.conflict1` 처럼 이름을 바꿔 남긴다.
//   - 한 번에 절반 넘게 지워지면 rclone 이 멈춘다 (실수로 폴더를 비웠을 때를 막는다).
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/afy0817/GodseonSync/internal/buildinfo"
	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/rclone"
	"github.com/afy0817/GodseonSync/internal/storage"
	"github.com/afy0817/GodseonSync/internal/store"
)

// Options 는 한 번의 동기화 실행 방법이다.
type Options struct {
	Folders []string // 이 ID(또는 이름)의 폴더만. 비우면 전체
	Resync  bool     // 처음처럼 양쪽을 합친다 (지우지 않음)
	Force   bool     // 한 번에 많이 지워져도 멈추지 않는다
	DryRun  bool     // 실제로 바꾸지 않고 바뀔 내용만 로그에 남긴다
}

// Result 는 동기화 결과다.
type Result struct {
	Failed     []string // 실패한 폴더 이름
	NeedChoice []string // 양쪽에 파일이 있어 어느 쪽으로 맞출지 골라야 하는 폴더 이름
	Synced     int      // 동기화한 폴더 수
	Skipped    bool     // 다른 동기화가 실행 중이라 건너뜀
}

// 지금 이 프로세스에서 돌고 있는 동기화 (Interrupt 로 멈춘다)
var (
	runMu     sync.Mutex
	runCancel context.CancelFunc
)

// Interrupt 는 이 프로세스에서 돌고 있는 동기화를 안전하게 멈춘다 (GodseonSync 를 끌 때).
// 지금 폴더의 rclone 에 중단 신호를 보내고, 남은 폴더는 건너뛴다.
func Interrupt() {
	runMu.Lock()
	defer runMu.Unlock()
	if runCancel != nil {
		runCancel()
	}
}

// interruptedMark 는 예전 버전이 끄느라 멈춘 폴더의 오류 기록 앞에 붙이던 말이다 (지금은 FolderStatus.Interrupted).
const interruptedMark = "중간에 멈춤"

// wasInterrupted 는 지난번에 끄느라 중간에 멈춘 폴더인지다.
func wasInterrupted(fst *store.FolderStatus) bool {
	return fst.Interrupted || strings.HasPrefix(fst.LastError, interruptedMark)
}

// ErrNotConfigured 는 아직 처음 설정을 하지 않았다는 뜻이다.
var ErrNotConfigured = i18n.Error("아직 설정되지 않았습니다. GodseonSync 화면에서 구글 드라이브를 연결하세요")

// NewDrive 는 로컬 설정대로 드라이브 접근 객체를 만든다.
func NewDrive(s store.Settings) *config.Drive {
	return config.NewDrive(rclone.New(), s.Remote, config.RootFolder)
}

// CachedConfig 는 이 PC 가 드라이브에서 마지막으로 받아 둔 설정이다 (없으면 nil).
func CachedConfig() *config.Config {
	c := &config.Config{}
	if store.ReadJSON(store.CacheFile(), c) != nil || c.Normalize() != nil {
		return nil
	}
	return c
}

// StatusInfo 는 드라이브 GodseonSync/.status/<PC ID>.json 에 올리는 이 PC 의 상태다.
func StatusInfo(s store.Settings, st *store.Status) config.Status {
	host, _ := os.Hostname()
	folders := map[string]config.FolderReport{}
	for id, f := range st.Folders {
		folders[id] = config.FolderReport{LastRun: f.LastRun, LastOK: f.LastOK, LastError: f.LastError}
	}
	return config.Status{
		ID: s.ClientID, Hostname: host, AppVersion: buildinfo.Version, LastSeen: store.Now(),
		Autostart: platform.AutostartEnabled(), Paused: s.Paused, IntervalMinutes: s.IntervalMinutes, Folders: folders,
	}
}

// PublishStatus 는 이 PC 상태를 드라이브에 올린다 (실패해도 동기화는 계속).
func PublishStatus(d *config.Drive, s store.Settings) {
	if err := d.SaveStatus(StatusInfo(s, store.LoadStatus())); err != nil {
		store.Log("PC 상태를 드라이브에 올리지 못함: %v", err)
	}
}

// Register 는 setting.json 의 PC 목록에 이 PC 가 없으면 넣고, 이름·OS 가 바뀌었으면 고친다.
// 다른 PC 에서 목록에서 뺐더라도 이 PC 에서 GodseonSync 가 돌면 다시 나타난다 (경로는 다시 입력해야 한다).
func Register(d *config.Drive, cfg *config.Config, s store.Settings) (*config.Config, error) {
	if pc := cfg.FindPC(s.ClientID); pc != nil && pc.Name == s.ClientName && pc.OS == platform.OSKey {
		return cfg, nil
	}
	return d.Update(s.ClientName, func(c *config.Config) error {
		pc := c.FindPC(s.ClientID)
		if pc == nil {
			c.PCs = append(c.PCs, config.PC{ID: s.ClientID, AddedAt: store.Now()})
			pc = &c.PCs[len(c.PCs)-1]
		}
		pc.Name, pc.OS = s.ClientName, platform.OSKey
		return nil
	})
}

// Run 은 동기화를 한 번 실행한다.
func Run(opt Options) (Result, error) {
	s := store.LoadSettings()
	if !s.Configured {
		return Result{}, ErrNotConfigured
	}
	lock, err := store.AcquireLock()
	if errors.Is(err, store.ErrBusy) {
		store.Log("다른 동기화가 실행 중이라 건너뜀")
		return Result{Skipped: true}, nil
	}
	if err != nil {
		return Result{}, err
	}
	defer lock.Release()
	ctx, cancel := context.WithCancel(context.Background())
	runMu.Lock()
	runCancel = cancel
	runMu.Unlock()
	defer func() {
		runMu.Lock()
		runCancel = nil
		runMu.Unlock()
		cancel()
	}()
	res, err := run(ctx, s, lock, opt)
	if err != nil {
		store.Log("동기화 중단: %v", err)
		store.AddNotice("err", i18n.T("동기화가 중단됐습니다"), err.Error())
		st := store.LoadStatus()
		st.LastRun, st.LastError = store.Now(), err.Error()
		_ = store.SaveStatus(st)
	}
	return res, err
}

func run(ctx context.Context, s store.Settings, lock *store.Lock, opt Options) (Result, error) {
	var res Result
	drive := NewDrive(s)
	// setting.json 이 없으면 만든다 (예전 형식 설정이 있으면 옮기고, 아니면 이 PC 가 마지막으로 받은 설정으로)
	cfg, _, err := drive.Ensure(s.ClientName, CachedConfig())
	if err != nil {
		return res, fmt.Errorf(i18n.T("드라이브에서 설정을 읽지 못했습니다: %w"), err)
	}
	if !opt.DryRun {
		if cfg, err = Register(drive, cfg, s); err != nil {
			return res, fmt.Errorf(i18n.T("드라이브의 PC 목록에 이 PC 를 올리지 못했습니다: %w"), err)
		}
	}
	_ = store.WriteJSON(store.CacheFile(), cfg)

	st := store.LoadStatus()
	st.LastError = ""
	for i := range cfg.Folders {
		f := &cfg.Folders[i]
		if len(opt.Folders) > 0 && !contains(opt.Folders, f.ID) && !contains(opt.Folders, f.Name) {
			continue
		}
		raw := f.Paths[s.ClientID]
		if raw == "" {
			continue // 이 PC 에서는 동기화하지 않는 폴더
		}
		if ctx.Err() != nil {
			store.Log("[%s] GodseonSync 를 끄는 중이라 건너뜀", f.Name)
			continue
		}
		fst := st.Folders[f.ID]
		if fst == nil {
			fst = &store.FolderStatus{}
			st.Folders[f.ID] = fst
		}
		lock.SetFolder(f.Name)
		res.Synced++
		if !syncFolder(ctx, drive, s, cfg, f, raw, fst, opt) {
			res.Failed = append(res.Failed, f.Name)
		} else if fst.NeedsChoice && !opt.DryRun {
			res.NeedChoice = append(res.NeedChoice, f.Name)
		}
		_ = store.SaveStatus(st) // 중간에 끊겨도 끝난 폴더 기록은 남긴다
	}
	// 설정에서 빠진 폴더 기록은 지운다
	known := map[string]bool{}
	for _, f := range cfg.Folders {
		known[f.ID] = true
	}
	for id := range st.Folders {
		if !known[id] {
			delete(st.Folders, id)
		}
	}
	st.LastRun = store.Now()
	_ = store.SaveStatus(st)
	if !opt.DryRun {
		PublishStatus(drive, s)
	}
	return res, nil
}

var errorPrefix = regexp.MustCompile(`^\s*ERROR\s*:\s*`)

func syncFolder(ctx context.Context, drive *config.Drive, s store.Settings, cfg *config.Config, f *config.Folder, raw string,
	fst *store.FolderStatus, opt Options) bool {
	name := f.Name
	local := platform.ExpandPath(raw)
	remote := drive.Spec(f.DrivePath)
	now := store.Now()
	fst.LastRun = now
	fail := func(msg string) bool {
		store.Log("[%s] 실패: %s", name, msg)
		if !opt.DryRun {
			fst.LastError, fst.LastErrorAt = msg, now
			fst.Interrupted, fst.TooManyDeletes = false, false
			store.AddNotice("err", i18n.T("[%s] 동기화 실패", name), msg)
		}
		return false
	}
	hadError := fst.LastError != "" && !wasInterrupted(fst)

	if dp := strings.ToLower(f.DrivePath); dp == "" || dp == strings.ToLower(drive.Root) || strings.HasPrefix(strings.ToLower(drive.Root), dp+"/") {
		return fail(i18n.T("드라이브 경로가 비었거나 GodseonSync 폴더 자체(또는 그 상위)라서 동기화하지 않습니다. 드라이브 경로를 고치세요."))
	}
	if !filepath.IsAbs(local) {
		return fail(i18n.T("이 PC 경로가 전체 경로가 아닙니다: %s", raw))
	}
	if info, err := os.Stat(local); os.IsNotExist(err) {
		if pi, err := os.Stat(filepath.Dir(local)); err != nil || !pi.IsDir() {
			return fail(i18n.T("상위 폴더가 없습니다: %s", filepath.Dir(local)))
		}
		if err := os.Mkdir(local, 0o755); err != nil {
			return fail(i18n.T("폴더를 만들 수 없습니다: %v", err))
		}
	} else if err != nil {
		return fail(err.Error())
	} else if !info.IsDir() {
		return fail(i18n.T("폴더가 아닙니다: %s", local))
	}
	// rclone bisync 는 두 경로를 이어 붙여 작업 파일 이름을 만든다 (255바이트 제한).
	// 로컬 경로가 길면 데이터 폴더 안의 짧은 링크를 거쳐서 넘긴다.
	target := local
	if config.SessionNameBytes(local, remote) > 255 {
		link, err := shortLink(local, f.ID)
		if err != nil {
			return fail(i18n.T("경로가 너무 길어 짧은 링크를 만들려 했지만 실패했습니다: %v", err))
		}
		target = link
	}
	if config.SessionNameBytes(target, remote) > 255 {
		return fail(i18n.T("드라이브 경로가 너무 깁니다 (rclone 작업 파일 이름 255바이트 제한). 드라이브 경로를 줄이세요."))
	}

	filters := config.FiltersText(cfg, f)
	ffile := filepath.Join(store.FiltersDir(), f.ID+".txt")
	if old, err := os.ReadFile(ffile); err != nil || string(old) != filters {
		if err := os.WriteFile(ffile, []byte(filters), 0o644); err != nil {
			return fail(i18n.T("제외 규칙 파일을 쓸 수 없습니다: %v", err))
		}
	}
	sig := folderSig(target, local, remote, filters, cfg.Epoch)
	needResync := opt.Resync || fst.NeedsResync || !fst.Initialized || fst.Sig != sig
	if wasInterrupted(fst) && !opt.DryRun {
		cleanPartials(name, target, remote, drive)
	}
	modeFlags := storage.SyncFlags(drive.RC, drive.Remote) // 구글 드라이브면 --drive-skip-gdocs
	choice := ""
	if needResync {
		if choice = store.Choice(f.ID); choice == "" && opt.Resync {
			choice = store.ChoiceMerge // '합치기' 버튼은 합치기를 고른 것이다
		}
		if choice == "" {
			lf, err := listFiles(drive.RC, local, ffile, nil)
			if err != nil {
				return fail(i18n.T("이 PC 폴더의 파일 수를 세지 못했습니다: %v", err))
			}
			rf, err := listFiles(drive.RC, remote, ffile, modeFlags)
			if err != nil {
				return fail(i18n.T("드라이브 폴더의 파일 수를 세지 못했습니다: %v", err))
			}
			nl, nd := len(lf), len(rf)
			if nl > 0 && nd > 0 {
				diff := compareFiles(lf, rf)
				if !sameFiles(diff) {
					store.Log("[%s] 이 PC 와 드라이브가 달라 멈췄습니다 (이 PC 에만 %d개, 드라이브에만 %d개, 내용이 다른 파일 %d개). 설정 화면에서 어느 쪽으로 맞출지 고르세요.",
						name, diff.OnlyLocal, diff.OnlyDrive, diff.Different)
					if !opt.DryRun {
						fst.NeedsChoice, fst.LocalFiles, fst.DriveFiles, fst.Diff = true, nl, nd, &diff
						fst.LastError, fst.LastErrorAt = "", ""
						store.AddNotice("warn", i18n.T("[%s] 어느 쪽으로 맞출지 골라 주세요", name),
							i18n.T("이 PC 에만 %d개, 드라이브에만 %d개, 내용이 다른 파일 %d개가 있어 멈췄습니다. 폴더 화면에서 드라이브 → 이 PC, 이 PC → 드라이브, 합치기 중 하나를 고르세요.", diff.OnlyLocal, diff.OnlyDrive, diff.Different))
					}
					return true
				}
				store.Log("[%s] 이 PC 와 드라이브의 파일이 같아(%d개) 묻지 않고 합칩니다", name, nl)
			}
			choice = store.ChoiceMerge // 한쪽이 비었거나 양쪽이 같다: 고를 것 없이 합친다
			if !opt.DryRun {
				// 끝날 때까지 기록해 둔다 — 중간에 끊겨 반쯤 올라간 뒤에도 다시 묻지 않고 이어서 합치게
				_ = store.SetChoice(f.ID, choice)
			}
		}
		if !opt.DryRun {
			fst.NeedsChoice = false // 골랐다 (또는 한쪽이 비어 고를 필요가 없다)
		}
		if (choice == store.ChoiceDrive || choice == store.ChoiceLocal) && !opt.DryRun {
			if err := overwrite(ctx, drive, name, choice, local, remote, ffile, modeFlags); err != nil {
				if ctx.Err() != nil {
					store.Log("[%s] GodseonSync 를 끄느라 덮어쓰기를 멈췄습니다. 다음 동기화 때 다시 합니다 (백업은 남아 있습니다).", name)
					fst.LastError, fst.LastErrorAt, fst.Interrupted = i18n.T("중간에 멈춤 — 다음 동기화 때 다시 덮어씁니다"), now, true
					return false
				}
				return fail(err.Error())
			}
			// 덮어쓴 뒤에는 양쪽이 같으니, bisync 가 실패해도 다음에는 다시 덮어쓰지 않고 합치기만 한다
			_ = store.SetChoice(f.ID, store.ChoiceMerge)
		}
	} else if store.Choice(f.ID) != "" && !opt.DryRun {
		_ = store.SetChoice(f.ID, "") // 합치기가 필요 없어졌다
	}

	args := []string{"bisync", target, remote,
		"--workdir", store.WorkDir(), "--filters-file", ffile,
		"--create-empty-src-dirs", "--resilient", "--recover",
		"--conflict-resolve", "newer", "--conflict-loser", "num",
		"--max-lock", "10m",
		"--color", "NEVER", "-v", "--stats", "1m", "--stats-one-line"}
	args = append(args, modeFlags...)
	if runtime.GOOS == "darwin" {
		// 맥은 한글 파일 이름을 자모로 풀어(NFD) 저장하기도 한다. 드라이브에는 NFC 로 올려야 윈도우에서 안 깨진다.
		args = append(args, "--local-unicode-normalization")
	}
	if needResync {
		if err := drive.RC.Mkdir(remote); err != nil { // 합치기는 양쪽 폴더가 다 있어야 한다
			return fail(i18n.T("드라이브 폴더를 만들 수 없습니다: %v", err))
		}
		args = append(args, "--resync", "--resync-mode", "newer")
	}
	if opt.Force {
		args = append(args, "--force")
	}
	if opt.DryRun {
		args = append(args, "--dry-run")
	}
	note := ""
	if needResync {
		note += i18n.T(" (합치기)")
	}
	if opt.DryRun {
		note += i18n.T(" (미리보기)")
	}
	if opt.Force {
		note += " (--force)"
	}
	store.Log("[%s] %s <-> %s%s", name, local, remote, note)

	var tail []string
	code, err := drive.RC.Stream(ctx, func(line string) {
		line = rclone.StripTimestamp(line)
		if strings.TrimSpace(line) == "" {
			return
		}
		tail = append(tail, line)
		if len(tail) > 40 {
			tail = tail[len(tail)-40:]
		}
		store.Log("[%s]   %s", name, line)
	}, args...)
	if ctx.Err() != nil {
		// 끄느라 멈췄다. 기록(합치기 완료 여부)은 그대로 두고 다음 동기화 때 이어서 한다.
		// 실행 직전에 멈추면 err(context canceled)도 오므로 err 보다 먼저 본다.
		store.Log("[%s] GodseonSync 를 끄느라 중간에 멈췄습니다. 다음 동기화 때 이어서 합니다.", name)
		if !opt.DryRun {
			fst.LastError, fst.LastErrorAt, fst.Interrupted = i18n.T("중간에 멈춤 — 다음 동기화 때 이어서 합니다"), now, true
		}
		return false
	}
	if err != nil {
		return fail(i18n.T("rclone 을 실행할 수 없습니다: %v", err))
	}
	if opt.DryRun {
		store.Log("[%s] 미리보기 끝 (종료 코드 %d)", name, code)
		return code == 0
	}
	if code == 0 {
		*fst = store.FolderStatus{Initialized: true, Sig: sig, LastRun: now, LastOK: store.Now()}
		if needResync && choice != "" {
			_ = store.SetChoice(f.ID, "")
		}
		if hadError {
			store.AddNotice("ok", i18n.T("[%s] 다시 정상으로 동기화했습니다", name), "")
		}
		store.Log("[%s] 완료", name)
		return true
	}
	text := strings.Join(tail, "\n")
	msg := i18n.T("종료 코드 %d", code)
	for i := len(tail) - 1; i >= 0; i-- {
		if strings.Contains(tail[i], "ERROR") {
			msg = errorPrefix.ReplaceAllString(tail[i], "")
			break
		}
	}
	switch {
	case strings.Contains(text, "Must run --resync"):
		// 기록이 꼬여서 bisync 가 멈췄다. 합치기는 지우지 않으니 다음 실행에서 자동으로 합친다.
		fst.NeedsResync = true
		msg += i18n.T(" — 다음 동기화 때 자동으로 합치기(resync)를 합니다")
	case strings.Contains(strings.ToLower(text), "too many deletes"):
		msg = i18n.T("한 번에 너무 많은 파일이 지워져서 안전을 위해 멈췄습니다. 일부러 지운 거라면 '강제 실행', 아니라면 '합치기'로 지운 파일을 되살리세요.")
		ok := fail(msg)
		if !opt.DryRun {
			fst.TooManyDeletes = true // fail 이 지운 뒤에 남긴다
		}
		return ok
	}
	return fail(msg)
}

// BackupSuffix 는 덮어쓰기 전에 만드는 백업 폴더 이름 꼬리다 (`<이름>_backup_20261019133512`).
func BackupSuffix(t time.Time) string { return "_backup_" + t.Format("20060102150405") }

// overwrite 는 고른 쪽으로 다른 쪽을 덮어쓴다 (rclone sync — 제외 규칙에 걸린 파일은 건드리지 않는다).
// 덮어쓰기 전에 덮어쓸 쪽을 제외 규칙 없이 통째로 옆에 `<이름>_backup_<년월일시분초>` 로 복사해 둔다.
//   - drive: 드라이브 → 이 PC. 이 PC 폴더를 `<이 PC 경로>_backup_…` 로 복사한 뒤 드라이브 내용으로 맞춘다.
//   - local: 이 PC → 드라이브. 드라이브 폴더를 `<드라이브 경로>_backup_…` 로 복사한 뒤(드라이브 안에서 복사) 이 PC 내용으로 맞춘다.
func overwrite(ctx context.Context, drive *config.Drive, name, choice, local, remote, ffile string, modeFlags []string) error {
	suffix := BackupSuffix(time.Now())
	from, to, backup := remote, local, local+suffix
	toLabel := i18n.T("이 PC 폴더")
	if choice == store.ChoiceLocal {
		from, to, backup = local, remote, remote+suffix
		toLabel = i18n.T("드라이브 폴더")
	}
	run := func(what string, args ...string) error {
		code, err := drive.RC.Stream(ctx, func(line string) {
			if line = strings.TrimSpace(rclone.StripTimestamp(line)); line != "" {
				store.Log("[%s]   %s", name, line)
			}
		}, args...)
		switch {
		case err != nil:
			return fmt.Errorf(i18n.T("%s: rclone 을 실행할 수 없습니다: %v"), what, err)
		case ctx.Err() != nil:
			return ctx.Err()
		case code != 0:
			return fmt.Errorf(i18n.T("%s에 실패했습니다 (종료 코드 %d, 로그 참고)"), what, code)
		}
		return nil
	}
	common := append([]string{"--create-empty-src-dirs", "--color", "NEVER", "-v", "--stats", "1m", "--stats-one-line"}, modeFlags...)
	if runtime.GOOS == "darwin" {
		common = append(common, "--local-unicode-normalization")
	}
	store.Log("[%s] 덮어쓰기 전에 %s를 백업합니다: %s", name, toLabel, backup)
	if err := run(i18n.T("%s 백업", toLabel), append([]string{"copy", to, backup}, common...)...); err != nil {
		return err
	}
	store.Log("[%s] %s → %s 로 덮어씁니다", name, from, to)
	if err := run(i18n.T("덮어쓰기"), append([]string{"sync", from, to, "--filter-from", ffile}, common...)...); err != nil {
		return errors.New(i18n.T("%s (백업: %s)", err.Error(), backup))
	}
	store.Log("[%s] 덮어쓰기 끝. 백업: %s", name, backup)
	store.AddNotice("ok", i18n.T("[%s] 덮어쓰기를 마쳤습니다", name), i18n.T("%s → %s. 덮어쓰기 전 내용은 %s 에 백업했습니다.", from, to, backup))
	return nil
}

// cleanPartials 는 지난번에 중간에 멈춘 동기화가 남긴 rclone 임시 파일(*.<16진수>.partial)만 양쪽에서 지운다.
func cleanPartials(name, local, remote string, drive *config.Drive) {
	for _, p := range []string{local, remote} {
		if _, err := drive.RC.Run(10*time.Minute, nil, "delete", "--include", config.PartialPattern, p); err != nil {
			store.Log("[%s] 남은 임시 파일 정리 실패 (%s): %v", name, p, err)
		}
	}
	store.Log("[%s] 지난번에 멈춘 동기화가 남긴 임시 파일을 정리했습니다", name)
}

// shortLink 는 데이터 폴더 안에 실제 폴더를 가리키는 짧은 링크를 만든다
// (맥·리눅스는 심볼릭 링크, 윈도우는 관리자 권한이 필요 없는 디렉터리 정션).
func shortLink(local, id string) (string, error) {
	link := filepath.Join(store.Home(), "links", id)
	if cur, err := os.Readlink(link); err == nil && filepath.Clean(strings.TrimPrefix(cur, `\??\`)) == filepath.Clean(local) {
		return link, nil
	}
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return "", err
	}
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 && runtime.GOOS != "windows" {
			return "", fmt.Errorf(i18n.T("%s 가 링크가 아닙니다"), link)
		}
		if err := os.Remove(link); err != nil { // 링크만 지운다 (가리키던 폴더는 그대로)
			return "", err
		}
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "mklink", "/J", link, local)
		platform.HideWindow(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return link, nil
	}
	return link, os.Symlink(local, link)
}

// folderSig 는 폴더의 경로·제외 규칙(과 저장소 세대) 해시다. 지난 동기화 때와 다르면 합치기부터 한다.
// 세대가 비어 있으면 세대를 넣기 전과 같은 값이 나온다 (업데이트만으로 모든 폴더를 다시 합치지 않게).
func folderSig(target, local, remote, filters, epoch string) string {
	text := target + "\n" + local + "\n" + remote + "\n" + filters
	if epoch != "" {
		text += "\n" + epoch
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
