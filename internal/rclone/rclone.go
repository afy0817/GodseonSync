// Package rclone 은 내장 rclone 을 부르는 얇은 래퍼다.
//
// rclone 은 이 실행 파일 안에 들어 있고(rcloneembed), 여기서는 자기 자신을 `<실행 파일> rclone <인자>` 로
// 다시 실행한다. rclone 설정 파일(구글 로그인 토큰)은 GodseonSync 데이터 폴더의 rclone.conf 를 쓴다.
package rclone

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/store"
)

// Error 는 rclone 이 실패했을 때의 오류다.
type Error struct {
	Code   int
	Msg    string
	Output string
}

func (e *Error) Error() string { return e.Msg }

// 종료 코드 (rclone 문서의 Exit Code)
const (
	CodeDirNotFound  = 3
	CodeFileNotFound = 4
)

// Client 는 rclone 을 실행한다.
type Client struct {
	Exe    string
	Prefix []string // 자기 자신을 부를 때 ["rclone"]
	Config string
}

// New 는 내장 rclone 을 쓰는 Client 를 만든다.
// GODSEONSYNC_RCLONE 환경 변수에 rclone 실행 파일을 주면 그것을 쓴다 (문제 확인용).
func New() *Client {
	if exe := os.Getenv("GODSEONSYNC_RCLONE"); exe != "" {
		return &Client{Exe: exe, Config: store.RcloneConfig()}
	}
	return &Client{Exe: platform.Executable(), Prefix: []string{"rclone"}, Config: store.RcloneConfig()}
}

// connectTimeout 은 연결 하나(TCP 연결·TLS 악수)를 기다리는 시간이다. rclone 기본값 1분은 Cat 같은 짧은 명령의
// 시간 제한(1분)과 같아서, 로그인 직후 잠깐 생겼다 사라지는 경로로 나간 연결이 막히면 다시 시도할 틈 없이 시간 초과로 끝난다.
// 15초에 끊으면 rclone 이 새 연결로 다시 시도한다 (docs/lessons.md '로그인 직후 막힌 연결').
const connectTimeout = "15s"

func (c *Client) command(ctx context.Context, args ...string) *exec.Cmd {
	full := append(append([]string{}, c.Prefix...), "--config", c.Config, "--ask-password=false", "--contimeout", connectTimeout)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, c.Exe, full...)
	platform.HideWindow(cmd)
	return cmd
}

var timestamp = regexp.MustCompile(`^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d `)

// StripTimestamp 는 rclone 로그 줄 앞의 시각을 지운다.
func StripTimestamp(line string) string { return timestamp.ReplaceAllString(line, "") }

// LastError 는 rclone 출력에서 가장 마지막 오류 줄을 찾는다.
func LastError(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.Contains(l, "ERROR") || strings.Contains(l, "CRITICAL") || strings.Contains(l, "Failed") {
			return cleanError(l)
		}
	}
	if len(lines) > 0 {
		return cleanError(lines[len(lines)-1])
	}
	return ""
}

var errorPrefix = regexp.MustCompile(`^(ERROR|CRITICAL|NOTICE)\s*:\s*`)

func cleanError(l string) string {
	return errorPrefix.ReplaceAllString(StripTimestamp(strings.TrimSpace(l)), "")
}

// Result 는 rclone 한 번 실행한 결과다.
type Result struct {
	Stdout, Stderr string
	Code           int
}

// Err 는 종료 코드가 0 이 아니면 rclone 의 마지막 오류 줄을 담은 *Error 를 돌려준다.
func (r Result) Err(what string) error {
	if r.Code == 0 {
		return nil
	}
	msg := LastError(r.Stderr)
	if msg == "" {
		msg = i18n.T("rclone %s 실패 (종료 코드 %d)", what, r.Code)
	}
	return &Error{Code: r.Code, Msg: msg, Output: r.Stderr}
}

// LaunchError 는 rclone 프로세스를 띄우지 못했을 때 보여 줄 문구다. OS 보안 정책이 막은 것은 원인과 할 일을 알린다
// (영어 시스템 문구 'An Application Control policy has blocked this file' 만으로는 알기 어렵다).
func LaunchError(err error) string {
	if platform.LaunchBlocked(err) {
		return i18n.T("윈도우 스마트 앱 컨트롤(또는 앱 컨트롤 정책)이 GodseonSync 실행을 막았습니다. 코드 서명이 없는 앱이라 생깁니다 — README 의 '처음 실행할 때 경고가 뜨면' 을 보세요")
	}
	return i18n.T("rclone 을 실행할 수 없습니다: %v", err)
}

// Exec 는 rclone 을 실행한다. 실행 자체가 안 되거나 시간이 넘었을 때만 오류를 돌려준다.
func (c *Client) Exec(timeout time.Duration, input *string, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := c.command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if input != nil {
		cmd.Stdin = strings.NewReader(*input)
	}
	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() == context.DeadlineExceeded {
		return res, &Error{Code: -1, Msg: i18n.T("rclone %s 이(가) %s 안에 끝나지 않았습니다", args[0], timeout)}
	}
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return res, &Error{Code: -1, Msg: LaunchError(err)}
		}
		res.Code = ee.ExitCode()
	}
	return res, nil
}

// Run 은 rclone 을 실행하고 표준 출력을 돌려준다. 종료 코드가 0 이 아니면 *Error.
func (c *Client) Run(timeout time.Duration, input *string, args ...string) (string, error) {
	res, err := c.Exec(timeout, input, args...)
	if err != nil {
		return "", err
	}
	return res.Stdout, res.Err(args[0])
}

// Stream 은 출력을 한 줄씩 넘기며 실행하고 종료 코드를 돌려준다.
// ctx 가 취소되면 rclone 에 중단 신호를 보낸다 (맥·리눅스는 SIGINT 라서 bisync 가 상태를 저장하고 끝나며
// 다음 실행 때 --recover 로 이어 간다. 윈도우는 바로 끝낸다). 1분 안에 안 끝나면 강제로 끝낸다.
func (c *Client) Stream(ctx context.Context, onLine func(string), args ...string) (int, error) {
	cmd := c.command(ctx, args...)
	cmd.Cancel = func() error { return interrupt(cmd.Process) }
	cmd.WaitDelay = time.Minute
	r, w := io.Pipe()
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			onLine(strings.TrimRight(sc.Text(), "\r"))
		}
		_, _ = io.Copy(io.Discard, r)
		close(done)
	}()
	err := cmd.Wait()
	w.Close()
	<-done
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

func notFound(code int) bool { return code == CodeDirNotFound || code == CodeFileNotFound }

// IsNotFound 는 종료 코드가 '파일·폴더 없음' 인지 알려 준다.
func IsNotFound(code int) bool { return notFound(code) }

// Cat 은 파일 내용을 읽는다. 파일이나 폴더가 없으면 found=false.
func (c *Client) Cat(spec string) (text string, found bool, err error) {
	res, err := c.Exec(time.Minute, nil, "cat", spec)
	if err != nil {
		return "", false, err
	}
	if notFound(res.Code) {
		return "", false, nil
	}
	return res.Stdout, true, res.Err("cat")
}

// Write 는 텍스트를 파일로 올린다.
func (c *Client) Write(spec, text string) error {
	_, err := c.Run(time.Minute, &text, "rcat", spec)
	return err
}

// Mkdir 은 폴더를 만든다 (이미 있으면 그대로).
func (c *Client) Mkdir(spec string) error {
	_, err := c.Run(time.Minute, nil, "mkdir", spec)
	return err
}

// DeleteFile 은 파일 하나를 지운다 (없으면 그대로).
func (c *Client) DeleteFile(spec string) error {
	res, err := c.Exec(time.Minute, nil, "deletefile", spec)
	if err != nil || notFound(res.Code) {
		return err
	}
	return res.Err("deletefile")
}

// MoveDir 는 폴더를 옮기거나 이름을 바꾼다 (구글 드라이브는 서버 안에서 이름만 바뀐다).
func (c *Client) MoveDir(from, to string) error {
	_, err := c.Run(2*time.Minute, nil, "moveto", from, to)
	return err
}

// Purge 는 폴더와 그 안을 모두 지운다 (구글 드라이브는 휴지통으로). 없으면 그대로.
func (c *Client) Purge(spec string) error {
	// 없는 폴더를 purge 하면 저장소에 따라 '없음' 이 아닌 오류가 나서 먼저 확인한다
	if _, found, err := c.LsDirs(spec); err != nil || !found {
		return err
	}
	res, err := c.Exec(2*time.Minute, nil, "purge", spec)
	if err != nil || res.Code == CodeDirNotFound {
		return err
	}
	return res.Err("purge")
}

// Owner 는 파일 소유자다 (구글 드라이브는 계정 이메일). 저장소가 알려 주지 않으면 빈 값.
// 다시 연결할 때 같은 계정인지 비교하는 데만 쓴다.
func (c *Client) Owner(spec string) (string, error) {
	out, err := c.Run(time.Minute, nil, "lsjson", "--stat", "-M", spec)
	if err != nil {
		return "", err
	}
	var item struct {
		Metadata map[string]string `json:"Metadata"`
	}
	if err := json.Unmarshal([]byte(out), &item); err != nil {
		return "", fmt.Errorf(i18n.T("lsjson 결과를 읽을 수 없습니다: %v"), err)
	}
	return item.Metadata["owner"], nil
}

// LsDirs 는 하위 폴더 이름 목록이다. 폴더가 없으면 found=false.
func (c *Client) LsDirs(spec string) (dirs []string, found bool, err error) {
	res, err := c.Exec(time.Minute, nil, "lsf", "--dirs-only", "--max-depth", "1", spec)
	if err != nil {
		return nil, false, err
	}
	if res.Code == CodeDirNotFound {
		return nil, false, nil
	}
	if err := res.Err("lsf"); err != nil {
		return nil, false, err
	}
	for _, l := range strings.Split(res.Stdout, "\n") {
		if l = strings.TrimRight(strings.TrimSpace(l), "/"); l != "" {
			dirs = append(dirs, l)
		}
	}
	sort.Strings(dirs)
	return dirs, true, nil
}

// CopyToLocal 은 드라이브 폴더의 파일을 로컬로 받는다. 폴더가 없으면 found=false.
func (c *Client) CopyToLocal(spec, dest, include string) (bool, error) {
	res, err := c.Exec(2*time.Minute, nil, "copy", spec, dest, "--include", include)
	if err != nil {
		return false, err
	}
	if res.Code == CodeDirNotFound {
		return false, nil
	}
	return res.Code == 0, res.Err("copy")
}

// HasRemote 는 rclone 설정에 그 이름의 연결이 있는지 본다.
// rclone 을 실행하지 않고 설정 파일을 직접 읽는다 (설정 화면을 열 때마다 불려서 빨라야 한다).
// 환경 변수로 만든 연결(RCLONE_CONFIG_<이름>_TYPE, 테스트용)도 본다.
func (c *Client) HasRemote(name string) bool {
	_, ok := c.RemoteOptions(name)
	return ok
}

// RemoteOptions 는 rclone 설정 파일에 적힌 연결의 항목들(type, scope, client_id …)이다. 연결이 없으면 ok=false.
// 토큰·비밀번호도 들어 있으니 화면에 그대로 보내지 않는다 (보여도 되는 값은 storage.Mode.Info 가 고른다).
// 환경 변수로 만든 연결은 type 만 알려 준다.
func (c *Client) RemoteOptions(name string) (opts map[string]string, ok bool) {
	env := "RCLONE_CONFIG_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_TYPE"
	if t := os.Getenv(env); t != "" {
		return map[string]string{"type": t}, true
	}
	b, err := os.ReadFile(c.Config)
	if err != nil {
		return nil, false
	}
	section := ""
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			section = l[1 : len(l)-1]
			if section == name {
				opts, ok = map[string]string{}, true
			}
			continue
		}
		if section != name || l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";") {
			continue
		}
		if k, v, found := strings.Cut(l, "="); found {
			opts[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return opts, ok
}

// SetOptions 는 rclone 설정 파일의 연결 name 칸에 키를 쓴다 (있으면 바꾼다). rclone 이 모르는 GodseonSync 전용 키
// (gs_path: 저장소 안의 기본 위치)를 연결과 함께 두는 데 쓴다 — rclone 은 모르는 키를 무시하고, 다시 쓸 때도 남긴다.
func (c *Client) SetOptions(name string, kv map[string]string) error {
	b, err := os.ReadFile(c.Config)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	var out []string
	section, found := "", false
	flush := func() {
		for k, v := range kv {
			out = append(out, k+" = "+v)
		}
		kv = nil
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			if section == name {
				flush()
			}
			section = t[1 : len(t)-1]
			found = found || section == name
		} else if section == name {
			if k, _, ok := strings.Cut(t, "="); ok {
				if v, mine := kv[strings.TrimSpace(k)]; mine {
					out = append(out, strings.TrimSpace(k)+" = "+v)
					delete(kv, strings.TrimSpace(k))
					continue
				}
			}
		}
		out = append(out, l)
	}
	if section == name {
		flush()
	}
	if !found {
		return fmt.Errorf(i18n.T("rclone 설정에 %s 연결이 없습니다"), name)
	}
	return store.WriteFile(c.Config, []byte(strings.Join(out, "\n")+"\n"))
}

// ReplaceRemote 는 연결 from 을 to 라는 이름으로 바꾼다. to 가 이미 있으면 그 연결은 지운다.
// rclone 에는 이름 바꾸기 명령이 없어서 설정 파일을 직접 고친다 (임시 파일에 쓴 뒤 바꿔치기).
// 새로 로그인한 연결을 임시 이름으로 만들었다가, 확인이 끝난 뒤 원래 이름에 넣을 때 쓴다.
func (c *Client) ReplaceRemote(from, to string) error {
	b, err := os.ReadFile(c.Config)
	if err != nil {
		return err
	}
	var out []string
	section, found := "", false
	for _, l := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = t[1 : len(t)-1]
			if section == from {
				found = true
				out = append(out, "["+to+"]")
				continue
			}
		}
		if section == to {
			continue
		}
		out = append(out, l)
	}
	if !found {
		return fmt.Errorf(i18n.T("rclone 설정에 %s 연결이 없습니다"), from)
	}
	return store.WriteFile(c.Config, []byte(strings.Join(out, "\n")))
}

// DeleteRemote 는 rclone 설정에서 연결을 지운다 (구글 로그인 해제).
func (c *Client) DeleteRemote(name string) error {
	_, err := c.Run(30*time.Second, nil, "config", "delete", name)
	return err
}

// ---------------------------------------------------------------- 구글 계정 연결

// ConnectJob 은 `rclone config create` 로 저장소를 연결한다.
// 구글 드라이브처럼 브라우저 로그인이 필요한 연결은 rclone 이 브라우저를 열어 로그인을 받는다.
type ConnectJob struct {
	mu      sync.Mutex
	Name    string
	State   string // running / done / error / cancelled
	URL     string // 브라우저가 안 열렸을 때 직접 여는 주소
	Message string // 실패 이유, 또는 로그인 뒤 확인하는 동안 보여 줄 말
	cancel  context.CancelFunc
	cleanup func()
	// finishing 은 로그인이 끝나 Finish 가 도는 중인지다. 이때는 취소하지 않는다
	// (임시 연결을 지우면 확인·바꿔 넣기가 중간에 깨진다).
	finishing bool
}

// ConnectHooks 는 연결 작업 앞뒤에 할 일이다.
type ConnectHooks struct {
	// Finish 는 rclone 이 연결을 만든 뒤 부른다. 오류를 돌려주면 연결 실패로 본다.
	// progress 로 화면에 보여 줄 말을 바꿀 수 있다.
	Finish func(progress func(string)) error
	// Cleanup 은 연결이 실패하거나 취소되면 부른다 (Finish 가 실패한 때도).
	Cleanup func()
}

var authURL = regexp.MustCompile(`(http://127\.0\.0\.1:\d+/auth\?state=\S+)`)

// StartConnect 는 `rclone config create <name> <typ> <params…>` 로 연결을 만든다.
// params 는 key=value 목록이다 (storage.Mode.Params 가 만든다).
func (c *Client) StartConnect(name, typ string, params []string, hooks ConnectHooks) (*ConnectJob, error) {
	args := append([]string{"config", "create", name, typ}, params...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	cmd := c.command(ctx, args...)
	cmd.Stdout = io.Discard // 표준 출력에는 토큰이 든 설정이 찍히므로 버린다
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	job := &ConnectJob{Name: name, State: "running", cancel: cancel, cleanup: hooks.Cleanup}
	go func() {
		var lines []string
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			l := sc.Text()
			lines = append(lines, l)
			if m := authURL.FindStringSubmatch(l); m != nil {
				job.mu.Lock()
				job.URL = m[1]
				job.mu.Unlock()
			}
		}
		err := cmd.Wait()
		cancel()
		if err == nil && hooks.Finish != nil && job.startFinish() {
			err = hooks.Finish(func(msg string) {
				job.mu.Lock()
				job.Message = msg
				job.mu.Unlock()
			})
		}
		if err != nil && job.cleanup != nil && !job.isCancelled() {
			job.cleanup() // 실패로 표시하기 전에 정리를 끝낸다 (곧바로 다시 연결해도 새 임시 연결을 지우지 않게)
		}
		job.mu.Lock()
		defer job.mu.Unlock()
		if job.State == "cancelled" {
			return // 정리는 Cancel 이 했다
		}
		if err == nil {
			job.State, job.Message = "done", ""
			return
		}
		job.State = "error"
		var re *exec.ExitError
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded): // 위에서 cancel() 했으므로 Canceled 와 구분한다
			job.Message = i18n.T("시간 안에 로그인하지 않아 취소했습니다")
		case errors.As(err, &re):
			job.Message = LastError(strings.Join(lines, "\n"))
		default:
			job.Message = err.Error() // Finish 가 돌려준 오류
		}
	}()
	return job, nil
}

// StartTask 는 rclone config create 없이 fn 만 도는 연결 작업이다 (저장해 둔 연결로 바꾸기처럼 로그인이 필요 없을 때).
// 화면은 같은 연결 작업 상태(running → done/error)로 따라간다. 취소는 받지 않는다.
func StartTask(name string, fn func(progress func(string)) error) *ConnectJob {
	job := &ConnectJob{Name: name, State: "running", cancel: func() {}, finishing: true}
	go func() {
		err := fn(func(msg string) {
			job.mu.Lock()
			job.Message = msg
			job.mu.Unlock()
		})
		job.mu.Lock()
		defer job.mu.Unlock()
		if err != nil {
			job.State, job.Message = "error", err.Error()
			return
		}
		job.State, job.Message = "done", ""
	}()
	return job
}

// startFinish 는 취소되지 않았으면 Finish 단계로 넘어간다 (그 뒤로는 Cancel 이 먹지 않는다).
func (j *ConnectJob) startFinish() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.State == "cancelled" {
		return false
	}
	j.finishing = true
	return true
}

// Cancel 은 로그인을 그만둔다.
// 로그인이 끝나 확인하는 중(finishing)이면 아무것도 하지 않는다. 정리(임시 연결 지우기)는 끝낸 뒤 돌아온다.
func (j *ConnectJob) Cancel() {
	j.mu.Lock()
	if j.State != "running" || j.finishing {
		j.mu.Unlock()
		return
	}
	j.State = "cancelled"
	j.cancel()
	j.mu.Unlock()
	if j.cleanup != nil {
		j.cleanup()
	}
}

func (j *ConnectJob) isCancelled() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.State == "cancelled"
}

// Snapshot 은 화면에 보낼 상태다. finishing 이면 화면은 취소 버튼을 숨긴다.
func (j *ConnectJob) Snapshot() map[string]string {
	j.mu.Lock()
	defer j.mu.Unlock()
	snap := map[string]string{"name": j.Name, "state": j.State, "url": j.URL, "message": j.Message}
	if j.finishing && j.State == "running" {
		snap["finishing"] = "1"
	}
	return snap
}
