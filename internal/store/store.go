// Package store 는 이 PC 전용 파일을 다룬다: 로컬 설정(settings.json), 동기화 상태(status.json),
// 로그, 동시 실행 잠금. 구글 드라이브에 올라가는 공용 설정은 config 패키지가 다룬다.
package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/platform"
)

// Home 은 데이터 폴더를 만들고 경로를 돌려준다.
func Home() string {
	d := platform.AppDir()
	_ = os.MkdirAll(d, 0o755)
	return d
}

func path(name string) string { return filepath.Join(Home(), name) }

func SettingsFile() string { return path("settings.json") }
func StatusFile() string   { return path("status.json") }
func CacheFile() string    { return path("config-cache.json") }
func LogFile() string      { return path("godseonsync.log") }
func LockFile() string     { return path("sync.lock") }
func UIFile() string       { return path("ui.json") }
func RcloneConfig() string { return path("rclone.conf") }
func WorkDir() string      { return path("bisync") }

// StatusesCacheFile 은 드라이브에서 마지막으로 받은 PC 별 상태(.status/*.json 모음)다.
// status.json(이 PC 의 동기화 기록)과 다른 파일이다.
func StatusesCacheFile() string { return path("statuses-cache.json") }

// FiltersDir 는 폴더별 rclone 제외 규칙 파일을 두는 곳이다.
func FiltersDir() string {
	d := path("filters")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// Now 는 시간대가 붙은 지금 시각이다 (다른 PC 에서 봐도 헷갈리지 않게).
func Now() string { return time.Now().Format(time.RFC3339) }

// NewID 는 짧은 무작위 ID 를 만든다.
func NewID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

// ReadJSON 은 파일을 v 로 읽는다. 파일이 없으면 os.ErrNotExist 를 감싼 오류.
func ReadJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// WriteJSON 은 임시 파일에 쓴 뒤 바꿔치기해서 중간에 끊겨도 파일이 깨지지 않게 한다.
func WriteJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(p, append(b, '\n'))
}

// WriteFile 은 WriteJSON 처럼 임시 파일에 쓴 뒤 바꿔치기한다 (권한 0600).
func WriteFile(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d.tmp", p, os.Getpid())
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	var err error
	for i := 0; ; i++ {
		err = os.Rename(tmp, p)
		if err == nil || i == 4 {
			return err
		}
		time.Sleep(200 * time.Millisecond) // 윈도우: 다른 프로세스가 잠깐 열고 있으면 실패한다
	}
}

// ---------------------------------------------------------------- 로컬 설정

// Settings 는 이 PC 에만 있는 설정이다.
type Settings struct {
	ClientID        string `json:"client_id"`        // 이 PC 의 고유 ID (공용 설정에서 PC 별 경로의 키)
	ClientName      string `json:"client_name"`      // 화면에 보이는 이름
	Remote          string `json:"rclone_remote"`    // rclone 설정 안의 구글 드라이브 연결 이름
	Root            string `json:"root"`             // 내 드라이브의 GodseonSync 폴더 (setting.json 이 있는 곳, 바꿀 수 없음)
	IntervalMinutes int    `json:"interval_minutes"` // 백그라운드 동기화 주기
	Paused          bool   `json:"paused"`           // 백그라운드 동기화 일시 정지
	Configured      bool   `json:"configured"`
	Language        string `json:"language,omitempty"` // 화면·알림 언어: ko / en (비면 설정 화면이 시스템 언어로 정해 저장한다)
}

var settingsMu sync.Mutex

// LoadSettings 는 로컬 설정을 읽는다. 처음이면 PC ID 와 이름을 만들어 저장한다.
func LoadSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	s := Settings{Remote: "gdrive", Root: "GodseonSync", IntervalMinutes: 10}
	_ = ReadJSON(SettingsFile(), &s)
	changed := false
	if s.ClientID == "" {
		s.ClientID = NewID("pc")
		changed = true
	}
	if s.ClientName == "" {
		s.ClientName = platform.Hostname()
		changed = true
	}
	if s.Remote == "" {
		s.Remote = "gdrive"
	}
	s.Root = "GodseonSync" // config.RootFolder (설정 위치는 고정)
	if s.IntervalMinutes <= 0 {
		s.IntervalMinutes = 10
	}
	i18n.SetLang(s.Language) // 설정을 읽을 때마다 알림·트레이·오류 문구 언어를 맞춘다
	if changed {
		_ = WriteJSON(SettingsFile(), s)
	}
	return s
}

// ApplyLanguage 는 로컬 설정의 언어만 읽어 문구 언어를 맞춘다 (설정 파일을 만들지 않는다 — 명령줄 시작용).
func ApplyLanguage() {
	var s struct {
		Language string `json:"language"`
	}
	_ = ReadJSON(SettingsFile(), &s)
	i18n.SetLang(s.Language)
}

// SaveSettings 는 로컬 설정을 저장한다.
func SaveSettings(s Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	i18n.SetLang(s.Language)
	return WriteJSON(SettingsFile(), s)
}

// ---------------------------------------------------------------- 동기화 상태

// FolderStatus 는 이 PC 에서 폴더 하나를 동기화한 기록이다.
type FolderStatus struct {
	Initialized bool   `json:"initialized"`            // 첫 합치기(resync)를 마쳤는지
	Sig         string `json:"sig,omitempty"`          // 경로·제외 규칙 해시. 바뀌면 다시 합친다
	NeedsResync bool   `json:"needs_resync,omitempty"` // rclone 이 기록 꼬임으로 멈췄을 때
	// Interrupted 는 GodseonSync 를 끄느라 중간에 멈췄다는 뜻이다 (다음 동기화 때 남은 임시 파일을 지우고 이어서 한다).
	// TooManyDeletes 는 한 번에 너무 많이 지워져 rclone 이 멈췄다는 뜻이다 (화면이 '강제 실행' 을 보여 준다).
	// 둘 다 오류 문구 대신 이 값으로 알아본다 (문구는 언어에 따라 바뀐다).
	Interrupted    bool `json:"interrupted,omitempty"`
	TooManyDeletes bool `json:"too_many_deletes,omitempty"`
	// NeedsChoice 는 합치기를 해야 하는데 이 PC 와 드라이브 양쪽에 파일이 있어서, 어느 쪽으로 맞출지
	// 사용자가 고를 때까지 이 폴더를 멈췄다는 뜻이다 (SetChoice). 그때 센 파일 수를 같이 남긴다.
	NeedsChoice bool        `json:"needs_choice,omitempty"`
	LocalFiles  int         `json:"local_files,omitempty"`
	DriveFiles  int         `json:"drive_files,omitempty"`
	Diff        *ChoiceDiff `json:"diff,omitempty"` // 양쪽 차이 (화면이 보여 준다)
	LastRun     string      `json:"last_run,omitempty"`
	LastOK      string      `json:"last_ok,omitempty"`
	LastError   string      `json:"last_error,omitempty"`
	LastErrorAt string      `json:"last_error_at,omitempty"`
}

// ChoiceDiff 는 합치기 전에 비교한 양쪽 차이다 (같은 경로·크기·수정 시각이면 같은 파일). 예시는 몇 개만.
type ChoiceDiff struct {
	OnlyLocal        int      `json:"only_local"` // 이 PC 에만 있는 파일 수
	OnlyDrive        int      `json:"only_drive"` // 저장소에만 있는 파일 수
	Different        int      `json:"different"`  // 양쪽에 있지만 크기나 수정 시각이 다른 파일 수
	OnlyLocalSamples []string `json:"only_local_samples,omitempty"`
	OnlyDriveSamples []string `json:"only_drive_samples,omitempty"`
	DifferentSamples []string `json:"different_samples,omitempty"`
}

// 양쪽에 파일이 있는 폴더를 어느 쪽으로 맞출지 (choices.json, 폴더 ID → 값)
const (
	ChoiceDrive = "drive" // 드라이브 → 이 PC 로 덮어쓰기 (이 PC 폴더를 먼저 백업)
	ChoiceLocal = "local" // 이 PC → 드라이브로 덮어쓰기 (드라이브 폴더를 먼저 백업)
	ChoiceMerge = "merge" // 합치기: 양쪽을 다 남기고 같은 파일은 최신 것 (지우지 않음)
)

var choicesMu sync.Mutex

func choicesFile() string { return path("choices.json") }

// Choice 는 그 폴더에 고른 값이다 (없으면 빈 값).
func Choice(folderID string) string {
	choicesMu.Lock()
	defer choicesMu.Unlock()
	m := map[string]string{}
	_ = ReadJSON(choicesFile(), &m)
	return m[folderID]
}

// SetChoice 는 그 폴더에 고른 값을 저장한다. 빈 값이면 지운다.
func SetChoice(folderID, choice string) error {
	choicesMu.Lock()
	defer choicesMu.Unlock()
	m := map[string]string{}
	_ = ReadJSON(choicesFile(), &m)
	if choice == "" {
		delete(m, folderID)
	} else {
		m[folderID] = choice
	}
	return WriteJSON(choicesFile(), m)
}

// Status 는 이 PC 의 동기화 기록 전체다.
type Status struct {
	Folders   map[string]*FolderStatus `json:"folders"`
	LastRun   string                   `json:"last_run,omitempty"`
	LastError string                   `json:"last_error,omitempty"`
}

// LoadStatus 는 동기화 기록을 읽는다.
func LoadStatus() *Status {
	st := &Status{}
	_ = ReadJSON(StatusFile(), st)
	if st.Folders == nil {
		st.Folders = map[string]*FolderStatus{}
	}
	return st
}

// SaveStatus 는 동기화 기록을 저장한다.
func SaveStatus(st *Status) error { return WriteJSON(StatusFile(), st) }

// ---------------------------------------------------------------- 로그

var logMu sync.Mutex

// Echo 가 참이면 로그를 표준 출력에도 찍는다 (명령줄에서 sync 를 돌릴 때).
var Echo = false

// Log 는 로그 파일에 한 줄 남긴다.
func Log(format string, args ...any) {
	line := time.Now().Format("2006-01-02 15:04:05") + " " + i18n.T(format, args...) // 문구는 이 PC 언어로
	logMu.Lock()
	defer logMu.Unlock()
	if Echo {
		fmt.Println(line)
	}
	f := LogFile()
	if st, err := os.Stat(f); err == nil && st.Size() > 5_000_000 {
		_ = os.Rename(f, f+".1")
	}
	fh, err := os.OpenFile(f, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	_, _ = fh.WriteString(line + "\n")
}

// ClearLog 는 로그 파일(과 예전 로그 .1)을 비운다.
func ClearLog(by string) error {
	logMu.Lock()
	f := LogFile()
	_ = os.Remove(f + ".1")
	err := os.WriteFile(f, nil, 0o644)
	logMu.Unlock()
	if err != nil {
		return err
	}
	Log("로그를 지웠습니다 (%s)", by)
	return nil
}

// LogTail 은 로그 마지막 n 줄이다.
func LogTail(n int) string {
	f, err := os.Open(LogFile())
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > 400_000 {
		_, _ = f.Seek(st.Size()-400_000, io.SeekStart)
	}
	data, _ := io.ReadAll(f)
	lines := strings.Split(strings.TrimRight(string(bytes.ToValidUTF8(data, []byte("?"))), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- 잠금

// LockStale 동안 잠금 파일이 갱신되지 않으면 비정상 종료로 본다.
const LockStale = 3 * time.Minute

// LockInfo 는 실행 중인 동기화 정보다.
type LockInfo struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
	Folder  string `json:"folder,omitempty"`
}

// Lock 은 동기화가 한 번에 하나만 돌게 한다 (화면에서 누른 동기화, 예약 동기화, 명령줄 sync 모두).
type Lock struct {
	mu   sync.Mutex
	info LockInfo
	stop chan struct{}
}

// ErrBusy 는 다른 동기화가 실행 중이라는 뜻이다.
var ErrBusy = i18n.Error("다른 동기화가 실행 중입니다")

// AcquireLock 은 잠금을 잡는다. 이미 실행 중이면 ErrBusy.
func AcquireLock() (*Lock, error) {
	p := LockFile()
	for i := 0; i < 2; i++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if !os.IsExist(err) {
				return nil, err
			}
			if ReadLock() != nil {
				return nil, ErrBusy
			}
			_ = os.Remove(p) // 오래된 잠금
			continue
		}
		f.Close()
		l := &Lock{info: LockInfo{PID: os.Getpid(), Started: Now()}, stop: make(chan struct{})}
		l.write()
		go l.beat()
		return l, nil
	}
	return nil, ErrBusy
}

// SetFolder 는 지금 동기화 중인 폴더 이름을 기록한다 (화면에 보여 준다).
func (l *Lock) SetFolder(name string) {
	l.mu.Lock()
	l.info.Folder = name
	l.mu.Unlock()
	l.write()
}

func (l *Lock) write() {
	l.mu.Lock()
	b, _ := json.Marshal(l.info)
	l.mu.Unlock()
	_ = os.WriteFile(LockFile(), b, 0o644)
}

func (l *Lock) beat() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			now := time.Now()
			_ = os.Chtimes(LockFile(), now, now)
		}
	}
}

// Release 는 잠금을 푼다.
func (l *Lock) Release() {
	close(l.stop)
	_ = os.Remove(LockFile())
}

// ReadLock 은 동기화가 실행 중이면 잠금 정보를, 아니면 nil 을 돌려준다.
func ReadLock() *LockInfo {
	p := LockFile()
	st, err := os.Stat(p)
	if err != nil || time.Since(st.ModTime()) > LockStale {
		return nil
	}
	info := &LockInfo{}
	_ = ReadJSON(p, info)
	return info
}
