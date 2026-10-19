// Package config 는 구글 드라이브에 올라가는 공용 설정을 다룬다.
//
// 드라이브 구조 (구글 로그인 후 자동으로 만든다):
//
//	내 드라이브/GodseonSync/setting.json           공용 설정: 폴더 목록, 드라이브 경로, PC 목록, PC 별 경로
//	내 드라이브/GodseonSync/.status/<PC ID>.json   PC 마다 마지막 동기화 결과 (각 PC 가 자기 파일만 쓴다)
//	내 드라이브/GodseonSync/<폴더 이름>/            동기화 폴더의 기본 위치
//
// PC 별 경로는 OS 가 아니라 PC ID 로 구분한다. 같은 윈도우라도 PC 가 다르면 경로를 따로 가진다.
// setting.json 은 revision 번호로 동시 수정을 막는다. 저장할 때 드라이브의 revision 이 읽은 시점과
// 다르면 다시 읽어서 바꿀 내용을 다시 적용한다 (Update). 화면에서 고친 폴더 목록은 그 사이에 다른 PC 가
// 폴더 목록을 바꿨을 때만 충돌로 본다.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/rclone"
	"github.com/afy0817/GodseonSync/internal/store"
)

const (
	// RootFolder 는 내 드라이브에서 GodseonSync 가 쓰는 폴더다.
	RootFolder = "GodseonSync"
	// SettingFile 은 RootFolder 안의 공용 설정 파일 이름이다.
	SettingFile = "setting.json"
	// StatusDir 은 RootFolder 안에서 PC 별 상태 파일을 두는 폴더 이름이다.
	StatusDir = ".status"
	// SchemaVersion 은 setting.json 형식 버전이다.
	SchemaVersion = 1
)

// DefaultExclude 는 처음 만들 때 넣는 공통 제외 규칙이다.
var DefaultExclude = []string{
	".DS_Store", "._*", "Thumbs.db", "desktop.ini", "~$*", ".~lock.*",
	"$RECYCLE.BIN/", "System Volume Information/",
}

// Folder 는 동기화 폴더 하나다.
type Folder struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	DrivePath string            `json:"drive_path"` // 내 드라이브 기준 경로 (예: GodseonSync/문서)
	Exclude   []string          `json:"exclude"`
	Paths     map[string]string `json:"paths"` // PC ID → 그 PC 의 로컬 경로
}

// PC 는 GodseonSync 를 쓰는 PC 하나다. OS 가 같아도 PC 마다 따로 등록된다.
type PC struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OS      string `json:"os,omitempty"` // mac / windows / linux
	AddedAt string `json:"added_at,omitempty"`
}

// Config 는 setting.json 전체다.
type Config struct {
	App       string `json:"app"`
	Version   int    `json:"version"`
	Revision  int    `json:"revision"`
	UpdatedAt string `json:"updated_at,omitempty"`
	UpdatedBy string `json:"updated_by,omitempty"`
	// Epoch 는 저장소의 GodseonSync 폴더를 새로 만든 때(권한 범위 옮기기 등)에 바꾸는 값이다.
	// 바뀌면 모든 PC 가 폴더마다 한 번 합친다(resync) — 비어 있는 새 폴더를 '파일이 지워졌다' 로 보지 않게.
	Epoch   string   `json:"epoch,omitempty"`
	Exclude []string `json:"exclude"`
	PCs     []PC     `json:"pcs"`
	Folders []Folder `json:"folders"`
}

// FindPC 는 ID 로 PC 를 찾는다 (없으면 nil).
func (c *Config) FindPC(id string) *PC {
	for i := range c.PCs {
		if c.PCs[i].ID == id {
			return &c.PCs[i]
		}
	}
	return nil
}

// FolderReport 는 PC 가 올리는 폴더별 최근 결과다.
type FolderReport struct {
	LastRun   string `json:"last_run,omitempty"`
	LastOK    string `json:"last_ok,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// Status 는 .status/<PC ID>.json 내용이다. 각 PC 가 동기화할 때마다 자기 파일만 덮어쓴다.
type Status struct {
	ID              string                  `json:"id"`
	Hostname        string                  `json:"hostname,omitempty"`
	AppVersion      string                  `json:"app_version,omitempty"`
	LastSeen        string                  `json:"last_seen,omitempty"`
	Autostart       bool                    `json:"autostart"`
	Paused          bool                    `json:"paused,omitempty"`
	IntervalMinutes int                     `json:"interval_minutes,omitempty"`
	Folders         map[string]FolderReport `json:"folders,omitempty"`
}

// ErrNewerSchema 는 더 새 버전 앱이 만든 설정이라는 뜻이다.
var ErrNewerSchema = errors.New(i18n.T("더 새 버전의 GodseonSync 가 만든 설정입니다. 이 PC 의 GodseonSync 를 업데이트하세요"))

// ErrNoSetting 은 드라이브에 setting.json 이 없다는 뜻이다.
var ErrNoSetting = errors.New(i18n.T("구글 드라이브에 GodseonSync/setting.json 이 없습니다"))

// ConflictError 는 다른 PC 가 먼저 같은 설정을 바꿨다는 뜻이다.
type ConflictError struct{ Current *Config }

func (e *ConflictError) Error() string {
	return i18n.T("다른 PC 에서 설정이 바뀌었습니다. 다시 불러온 뒤 저장하세요")
}

// ValidationError 는 저장하면 안 되는 설정이라는 뜻이다.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string { return strings.Join(e.Problems, "\n") }

var multiSlash = regexp.MustCompile(`/+`)

// NormDrivePath 는 드라이브 경로를 'a/b/c' 꼴로 맞춘다 (앞뒤 / 없음).
func NormDrivePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	return strings.Trim(multiSlash.ReplaceAllString(p, "/"), "/")
}

func cleanList(items []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" && !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}

// Default 는 새 설정이다.
func Default() *Config {
	c := &Config{Exclude: append([]string{}, DefaultExclude...)}
	_ = c.Normalize()
	return c
}

// Normalize 는 빈 값을 채우고 형식을 맞춘다.
func (c *Config) Normalize() error {
	if c.Version > SchemaVersion {
		return ErrNewerSchema
	}
	c.App, c.Version = "GodseonSync", SchemaVersion
	if c.Exclude == nil {
		c.Exclude = append([]string{}, DefaultExclude...)
	}
	c.Exclude = cleanList(c.Exclude)
	pcs := []PC{}
	seen := map[string]bool{}
	for _, p := range c.PCs {
		if p.ID != "" && !seen[p.ID] {
			seen[p.ID] = true
			p.Name = strings.TrimSpace(p.Name)
			pcs = append(pcs, p)
		}
	}
	c.PCs = pcs
	if c.Folders == nil {
		c.Folders = []Folder{}
	}
	for i := range c.Folders {
		f := &c.Folders[i]
		f.Name = strings.TrimSpace(f.Name)
		f.DrivePath = NormDrivePath(f.DrivePath)
		f.Exclude = cleanList(f.Exclude)
		paths := map[string]string{}
		for id, p := range f.Paths {
			if p = strings.TrimSpace(p); p != "" {
				paths[id] = p
			}
		}
		f.Paths = paths
	}
	return nil
}

// ---------------------------------------------------------------- 검사

var winAbs = regexp.MustCompile(`^([A-Za-z]:[\\/]|\\\\|//|%[^%]+%|~)`)

func isAbs(p, osKey string) bool {
	win := winAbs.MatchString(p)
	posix := strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.HasPrefix(p, "$")
	switch osKey {
	case "windows":
		return win
	case "mac", "linux":
		return posix
	}
	return win || posix
}

func pathKey(p, osKey string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/")
	if osKey == "windows" || osKey == "mac" { // 대소문자를 가리지 않는 파일 시스템
		p = strings.ToLower(p)
	}
	return p
}

func overlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// SessionNameBytes 는 rclone bisync 가 작업 폴더에 만드는 파일 이름 길이를 어림한다 (두 경로를 이어 붙인다).
func SessionNameBytes(local, remote string) int { return len(local) + len(remote) + 32 }

// Validate 는 저장을 막아야 하는 문제 목록을 돌려준다.
func Validate(c *Config) []string {
	var errs []string
	root := strings.ToLower(RootFolder)
	names := map[string]bool{}
	type labeled struct{ label, path string }
	var drivePaths []labeled
	for _, f := range c.Folders {
		label := f.Name
		if label == "" {
			label = i18n.T("(이름 없음)")
			errs = append(errs, i18n.T("이름이 비어 있는 폴더가 있습니다."))
		} else if names[strings.ToLower(f.Name)] {
			errs = append(errs, i18n.T("폴더 이름 '%s' 이(가) 겹칩니다.", f.Name))
		}
		names[strings.ToLower(f.Name)] = true
		dp := strings.ToLower(f.DrivePath)
		switch {
		case dp == "":
			errs = append(errs, i18n.T("[%s] 구글 드라이브 경로를 입력하세요.", label))
		case contains(strings.Split(dp, "/"), ".."):
			errs = append(errs, i18n.T("[%s] 드라이브 경로에 '..' 은 쓸 수 없습니다.", label))
		case dp == root || strings.HasPrefix(root, dp+"/"):
			errs = append(errs, i18n.T("[%s] %s 폴더 자체는 동기화할 수 없습니다. %s/%s 처럼 그 안의 폴더를 쓰세요.", label, RootFolder, RootFolder, f.Name))
		case dp == root+"/"+strings.ToLower(SettingFile) || overlap(dp, root+"/"+StatusDir):
			errs = append(errs, i18n.T("[%s] %s 는 GodseonSync 가 쓰는 이름이라 드라이브 경로로 쓸 수 없습니다.", label, f.DrivePath))
		default:
			for _, o := range drivePaths {
				if overlap(dp, o.path) {
					errs = append(errs, i18n.T("[%s] 드라이브 경로가 [%s] 와(과) 겹칩니다 (같거나 한쪽이 다른 쪽 안에 있음).", label, o.label))
				}
			}
			drivePaths = append(drivePaths, labeled{label, dp})
		}
	}
	// PC 별 경로: 전체 경로여야 하고, 한 PC 안에서 서로 겹치면 안 된다
	perPC := map[string][]labeled{}
	for _, f := range c.Folders {
		for id, p := range f.Paths {
			osKey, who := "", id
			if pc := c.FindPC(id); pc != nil {
				osKey, who = pc.OS, pc.Name
			}
			if !isAbs(p, osKey) {
				errs = append(errs, i18n.T("[%s] %s 의 경로 '%s' 는 전체 경로(절대 경로)여야 합니다.", f.Name, who, p))
				continue
			}
			key := pathKey(p, osKey)
			for _, o := range perPC[id] {
				if overlap(key, o.path) {
					errs = append(errs, i18n.T("[%s] %s 의 경로가 [%s] 와(과) 겹칩니다.", f.Name, who, o.label))
				}
			}
			perPC[id] = append(perPC[id], labeled{f.Name, key})
		}
	}
	return errs
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// FilterRule 은 제외 규칙 한 줄을 rclone 필터 문법으로 바꾼다.
func FilterRule(p string) string {
	switch {
	case strings.HasPrefix(p, "- ") || strings.HasPrefix(p, "+ "):
		return p // rclone 필터 문법 그대로
	case strings.HasSuffix(p, "/"):
		return "- " + p + "**"
	default:
		return "- " + p
	}
}

// PartialPattern 은 rclone 이 받는 중인 파일에 붙이는 임시 이름(이름.<16진수 8자리>.partial)이다.
// 동기화가 중간에 멈추면 남을 수 있어서 항상 동기화에서 빼고, 다음 동기화 때 지운다.
const PartialPattern = "*.[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f].partial"

// FiltersText 는 폴더 하나의 rclone 필터 파일 내용이다.
func FiltersText(c *Config, f *Folder) string {
	lines := []string{"# GodseonSync 가 만든 파일 — 직접 고치지 마세요", "- " + PartialPattern}
	for _, p := range append(append([]string{}, c.Exclude...), f.Exclude...) {
		lines = append(lines, FilterRule(p))
	}
	return strings.Join(lines, "\n") + "\n"
}

// ---------------------------------------------------------------- 드라이브 읽기·쓰기

// Drive 는 드라이브의 GodseonSync 폴더(setting.json, .status)를 읽고 쓴다.
type Drive struct {
	RC     *rclone.Client
	Remote string
	Base   string // 연결의 기본 위치 (BaseOf, 보통 빈 값)
	Root   string
}

// NewDrive 는 Drive 를 만든다. root 는 보통 RootFolder 다 (테스트에서만 바꾼다).
func NewDrive(rc *rclone.Client, remote, root string) *Drive {
	if root = NormDrivePath(root); root == "" {
		root = RootFolder
	}
	return &Drive{RC: rc, Remote: remote, Base: BaseOf(rc, remote), Root: root}
}

// Spec 은 rclone 에 넘기는 '연결이름:경로' 문자열이다.
func (d *Drive) Spec(p string) string { return Spec(d.Remote, d.Base, p) }

// Spec 은 연결 remote 의 기본 위치 base(빈 값이면 저장소 맨 위) 아래 경로 p 다 (rclone 경로).
func Spec(remote, base, p string) string {
	p = NormDrivePath(p)
	switch {
	case base == "":
		return remote + ":" + p
	case p == "":
		return remote + ":" + base
	}
	return remote + ":" + base + "/" + p
}

// BaseOf 는 연결의 기본 위치다 (rclone.conf 의 gs_path — SFTP 처럼 홈 폴더 맨 위에 쓸 수 없는 서버용).
func BaseOf(rc *rclone.Client, remote string) string {
	o, _ := rc.RemoteOptions(remote)
	return strings.TrimRight(strings.TrimSpace(o["gs_path"]), "/")
}

// SettingPath 는 setting.json 의 드라이브 경로다.
func (d *Drive) SettingPath() string { return d.Root + "/" + SettingFile }

// writeMu 는 한 프로그램 안에서 setting.json 을 만들고 고치는 일이 동시에 돌지 않게 한다
// (설정 창과 예약 동기화가 거의 같은 때 Ensure 를 부르면 같은 이름 파일이 두 개 생길 수 있다).
var writeMu sync.Mutex

// NotFoundRetry 는 setting.json 이 없다고 확정하기 전에 다시 확인하는 횟수와 간격이다.
// 구글 드라이브는 방금 만든 파일을 몇 초 동안 못 찾을 때가 있어서(검색 색인 지연), 한 번 못 찾았다고
// 새로 만들면 같은 이름 파일이 두 개 생긴다.
var NotFoundRetry = struct {
	Times int
	Delay time.Duration
}{2, 2 * time.Second}

// Load 는 setting.json 을 읽는다. 없으면 (다시 확인한 뒤) ErrNoSetting.
func (d *Drive) Load() (*Config, error) { return d.load(true) }

func (d *Drive) load(recheck bool) (*Config, error) {
	for i := 0; ; i++ {
		res, err := d.RC.Exec(time.Minute, nil, "cat", d.Spec(d.SettingPath()))
		if err != nil {
			return nil, err
		}
		if res.Code == 0 {
			if strings.Contains(res.Stderr, "Duplicate object") {
				d.dedupe()
			}
			return parse(res.Stdout)
		}
		if !rclone.IsNotFound(res.Code) {
			return nil, res.Err("cat")
		}
		if !recheck || i >= NotFoundRetry.Times {
			return nil, ErrNoSetting
		}
		time.Sleep(NotFoundRetry.Delay)
	}
}

// dedupe 는 GodseonSync 폴더 바로 아래에 같은 이름 파일이 여러 개 생겼을 때 가장 최근 것만 남긴다
// (구글 드라이브는 같은 이름을 허용한다). 같은 이름 폴더는 하나로 합친다.
func (d *Drive) dedupe() {
	if _, err := d.RC.Run(2*time.Minute, nil, "dedupe", "--dedupe-mode", "newest", "--max-depth", "1", d.Spec(d.Root)); err != nil {
		store.Log("드라이브 %s 의 중복 파일 정리 실패: %v", d.Root, err)
		return
	}
	store.Log("드라이브 %s 에 같은 이름 파일이 여러 개 있어 가장 최근 것만 남겼습니다", d.Root)
}

func parse(text string) (*Config, error) {
	c := &Config{}
	if err := json.Unmarshal([]byte(text), c); err != nil {
		return nil, fmt.Errorf(i18n.T("드라이브의 setting.json 이 손상되었습니다 (%v)"), err)
	}
	if err := c.Normalize(); err != nil {
		return nil, err
	}
	return c, nil
}

// Ensure 는 setting.json 을 읽고, 없으면 만든다. 만들 때는 예전 형식 설정(.godseonsync)이 있으면 그것을 옮기고,
// 없으면 fallback(이 PC 가 마지막으로 받아 둔 설정)을, 그것도 없으면 빈 설정을 쓴다.
// created 는 이번에 새로 만들었는지다.
func (d *Drive) Ensure(by string, fallback *Config) (c *Config, created bool, err error) {
	if c, err = d.Load(); err == nil || !errors.Is(err, ErrNoSetting) {
		return c, false, err // 대부분은 여기서 끝난다 (잠금 없이)
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	c, err = d.Load() // 잠금을 기다리는 동안 다른 쪽이 만들었을 수 있다
	if err == nil || !errors.Is(err, ErrNoSetting) {
		return c, false, err
	}
	c, err = d.loadLegacy()
	if err != nil {
		return nil, false, err
	}
	switch {
	case c != nil:
		// 예전 설정은 검사 없이 그대로 옮긴다 (문제 있는 폴더는 엔진이 건너뛰고 설정 창이 알려 준다)
		if err := d.RC.Mkdir(d.Spec(d.Root)); err != nil {
			return nil, false, err
		}
		if c, err = d.save(c, 0, by, false); err != nil {
			return nil, false, err
		}
		d.afterCreate()
		store.Log("예전 형식 설정(%s/.godseonsync)을 %s 로 옮겼습니다. 예전 폴더는 드라이브에서 지워도 됩니다.", d.Root, d.SettingPath())
		return c, true, nil
	case fallback != nil && len(Validate(fallback)) == 0:
		cp := *fallback
		c = &cp
		store.Log("드라이브에 %s 이 없어 이 PC 가 마지막으로 받은 설정으로 다시 만듭니다", d.SettingPath())
	default:
		c = Default()
	}
	c, err = d.create(c, by)
	return c, err == nil, err
}

// Create 는 GodseonSync 폴더를 만들고 c 를 새 setting.json 으로 올린다 (이미 있으면 *ConflictError).
func (d *Drive) Create(c *Config, by string) (*Config, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	return d.create(c, by)
}

func (d *Drive) create(c *Config, by string) (*Config, error) {
	if err := c.Normalize(); err != nil {
		return nil, err
	}
	if errs := Validate(c); len(errs) > 0 {
		return nil, &ValidationError{Problems: errs}
	}
	if err := d.RC.Mkdir(d.Spec(d.Root)); err != nil {
		return nil, err
	}
	saved, err := d.save(c, 0, by, false)
	if err == nil {
		d.afterCreate()
	}
	return saved, err
}

// afterCreate 는 setting.json 을 처음 쓴 직후 목록을 보고, 다른 PC 가 거의 같은 때 만들어서 같은 이름 파일이
// 여러 개 생겼으면 가장 최근 것만 남긴다 (드라이브 색인이 늦으면 못 볼 수도 있어 Load 에서도 한 번 더 본다).
func (d *Drive) afterCreate() {
	out, err := d.RC.Run(time.Minute, nil, "lsf", "--files-only", "--max-depth", "1", "--include", "/"+SettingFile, d.Spec(d.Root))
	if err == nil && strings.Count(out, SettingFile) > 1 {
		d.dedupe()
	}
}

// loadLegacy 는 0.1.0 개발판이 쓰던 GodseonSync/.godseonsync/config.json 을 읽어 새 형식으로 바꾼다.
func (d *Drive) loadLegacy() (*Config, error) {
	text, found, err := d.RC.Cat(d.Spec(d.Root + "/.godseonsync/config.json"))
	if err != nil || !found {
		return nil, err
	}
	c, err := parse(text)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "godseonsync-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if ok, err := d.RC.CopyToLocal(d.Spec(d.Root+"/.godseonsync/clients"), tmp, "*.json"); err == nil && ok {
		files, _ := filepath.Glob(filepath.Join(tmp, "*.json"))
		for _, f := range files {
			var old PC
			if store.ReadJSON(f, &old) == nil && old.ID != "" && c.FindPC(old.ID) == nil {
				c.PCs = append(c.PCs, PC{ID: old.ID, Name: old.Name, OS: old.OS, AddedAt: store.Now()})
			}
		}
	}
	return c, nil
}

// Save 는 setting.json 을 올린다. 드라이브의 revision 이 baseRevision 과 다르면 *ConflictError.
func (d *Drive) Save(c *Config, baseRevision int, by string) (*Config, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	return d.save(c, baseRevision, by, baseRevision != 0)
}

// save 의 recheck 는 '없음' 을 다시 확인할지다 (Ensure·Create 는 이미 확인했으므로 끈다).
func (d *Drive) save(c *Config, baseRevision int, by string, recheck bool) (*Config, error) {
	current, err := d.load(recheck)
	if err != nil && !errors.Is(err, ErrNoSetting) {
		return nil, err
	}
	currentRev := 0
	if current != nil {
		currentRev = current.Revision
	}
	if currentRev != baseRevision {
		return nil, &ConflictError{Current: current}
	}
	if err := c.Normalize(); err != nil {
		return nil, err
	}
	for i := range c.Folders {
		if c.Folders[i].ID == "" {
			c.Folders[i].ID = store.NewID("f")
		}
	}
	next := currentRev + 1
	if current == nil && c.Revision >= next {
		next = c.Revision + 1 // 새로 만들 때(옮기기, 이 PC 의 마지막 설정으로 다시 만들기)는 번호를 이어 간다
	}
	c.Revision, c.UpdatedAt, c.UpdatedBy = next, store.Now(), by
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := d.RC.Write(d.Spec(d.SettingPath()), string(b)+"\n"); err != nil {
		return nil, err
	}
	return c, nil
}

// Update 는 setting.json 을 읽어 fn 으로 고친 뒤 저장한다. 그 사이 다른 PC 가 저장했으면 다시 읽어 fn 을
// 다시 적용한다 (fn 이 *ConflictError 를 돌려주면 그대로 실패한다).
func (d *Drive) Update(by string, fn func(c *Config) error) (*Config, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		c, err := d.Load()
		if err != nil {
			return nil, err
		}
		if err := fn(c); err != nil {
			return nil, err
		}
		if err := c.Normalize(); err != nil {
			return nil, err
		}
		if errs := Validate(c); len(errs) > 0 {
			return nil, &ValidationError{Problems: errs}
		}
		saved, err := d.save(c, c.Revision, by, true)
		var ce *ConflictError
		if errors.As(err, &ce) {
			lastErr = err
			continue
		}
		return saved, err
	}
	return nil, lastErr
}

// SameFolders 는 두 설정의 폴더 목록과 공통 제외 규칙이 같은지 본다 (PC 목록 등은 보지 않는다).
func SameFolders(a, b *Config) bool {
	return reflect.DeepEqual(a.Folders, b.Folders) && reflect.DeepEqual(a.Exclude, b.Exclude)
}

// LoadStatuses 는 .status/*.json 을 모두 읽는다.
func (d *Drive) LoadStatuses() (map[string]Status, error) {
	out := map[string]Status{}
	tmp, err := os.MkdirTemp("", "godseonsync-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(tmp)
	found, err := d.RC.CopyToLocal(d.Spec(d.Root+"/"+StatusDir), tmp, "*.json")
	if err != nil || !found {
		return out, err
	}
	files, _ := filepath.Glob(filepath.Join(tmp, "*.json"))
	for _, f := range files {
		var st Status
		if store.ReadJSON(f, &st) != nil {
			continue
		}
		if st.ID == "" {
			st.ID = strings.TrimSuffix(filepath.Base(f), ".json")
		}
		out[st.ID] = st
	}
	return out, nil
}

// SaveStatus 는 .status/<ID>.json 을 올린다.
func (d *Drive) SaveStatus(st Status) error {
	b, _ := json.MarshalIndent(st, "", "  ")
	return d.RC.Write(d.Spec(d.Root+"/"+StatusDir+"/"+st.ID+".json"), string(b)+"\n")
}

// DeleteStatus 는 .status/<ID>.json 을 지운다.
func (d *Drive) DeleteStatus(id string) error {
	return d.RC.DeleteFile(d.Spec(d.Root + "/" + StatusDir + "/" + id + ".json"))
}
