// Package app 은 명령줄 진입점이다.
package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/afy0817/GodseonSync/internal/agent"
	"github.com/afy0817/GodseonSync/internal/buildinfo"
	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/engine"
	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/platform"
	"github.com/afy0817/GodseonSync/internal/store"
	"github.com/afy0817/GodseonSync/internal/window"
)

const usage = `GodseonSync — 여러 PC 의 폴더를 구글 드라이브를 거쳐 양방향 동기화

사용법:
  GodseonSync                    실행 (트레이 아이콘 + 설정 화면)
  GodseonSync sync [폴더 ...]     지금 동기화
        --dry-run   실제로 바꾸지 않고 바뀔 내용만 로그에 남김
        --resync    처음처럼 양쪽을 합침 (지우지 않음)
        --force     한 번에 많이 지워져도 멈추지 않음
  GodseonSync status             설정·마지막 결과 보기
  GodseonSync log [-n 100]       최근 로그 보기 (--clear 로그 지우기)
  GodseonSync autostart on|off   로그인할 때 자동 실행 켜기/끄기
  GodseonSync quit               실행 중인 GodseonSync 끝내기
  GodseonSync version            버전 보기
  GodseonSync rclone ...         내장 rclone 직접 실행 (문제 확인용)
`

// Main 은 명령을 실행하고 종료 코드를 돌려준다.
func Main(args []string) int {
	store.ApplyLanguage() // 명령줄 출력도 이 PC 언어로
	// 맥 Finder 가 예전에는 -psn_ 인자를 붙였다
	var clean []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-psn_") {
			clean = append(clean, a)
		}
	}
	args = clean
	cmd := "open"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	if cmd != "open" && cmd != "agent" && cmd != "window" {
		platform.AttachParentConsole() // 윈도우: 명령 프롬프트에서 실행했을 때 출력이 보이게
	}
	var err error
	switch cmd {
	case "open":
		err = agent.Run(agent.Options{OpenUI: true})
	case "agent":
		fs := flag.NewFlagSet("agent", flag.ContinueOnError)
		background := fs.Bool("background", false, i18n.T("설정 화면을 열지 않음 (로그인 자동 실행)"))
		noTray := fs.Bool("no-tray", false, i18n.T("트레이 아이콘 없이 실행"))
		if err := fs.Parse(args); err != nil {
			return 2
		}
		err = agent.Run(agent.Options{OpenUI: !*background, NoTray: *noTray})
	case "window":
		err = runWindow()
	case "sync":
		return cmdSync(args)
	case "status":
		return cmdStatus()
	case "log":
		fs := flag.NewFlagSet("log", flag.ContinueOnError)
		n := fs.Int("n", 100, i18n.T("줄 수"))
		clear := fs.Bool("clear", false, i18n.T("로그 지우기"))
		if err := fs.Parse(args); err != nil {
			return 2
		}
		if *clear {
			if err = store.ClearLog(i18n.T("명령줄")); err == nil {
				fmt.Println(i18n.T("로그를 지웠습니다."))
			}
			break
		}
		fmt.Println(store.LogTail(*n))
	case "autostart":
		if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
			fmt.Fprintln(os.Stderr, i18n.T("사용법: GodseonSync autostart on|off"))
			return 2
		}
		err = platform.SetAutostart(args[0] == "on")
		if err == nil {
			fmt.Println(i18n.T("로그인 자동 실행:"), map[bool]string{true: i18n.T("켜짐"), false: i18n.T("꺼짐")}[platform.AutostartEnabled()])
		}
	case "quit":
		err = quitRunning()
	case "version", "--version", "-v":
		fmt.Println("GodseonSync", buildinfo.Version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, i18n.T("알 수 없는 명령: %s", cmd)+"\n\n"+usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("오류:"), err)
		return 1
	}
	return 0
}

func cmdSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "")
	resync := fs.Bool("resync", false, "")
	force := fs.Bool("force", false, "")
	// 폴더 이름과 옵션이 섞여 있어도 받는다
	var folders []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		rest = fs.Args()
		if len(rest) > 0 {
			folders = append(folders, rest[0])
			rest = rest[1:]
		}
	}
	store.Echo = true
	res, err := engine.Run(engine.Options{Folders: folders, Resync: *resync, Force: *force, DryRun: *dry})
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("오류:"), err)
		return 1
	}
	if res.Skipped {
		return 0
	}
	if len(res.Failed) > 0 {
		fmt.Fprintln(os.Stderr, i18n.T("실패한 폴더:"), strings.Join(res.Failed, ", "))
		return 1
	}
	return 0
}

func cmdStatus() int {
	s := store.LoadSettings()
	st := store.LoadStatus()
	fmt.Println(i18n.T("GodseonSync %s   OS: %s   이 PC: %s (%s)", buildinfo.Version, platform.OSKey, s.ClientName, s.ClientID))
	fmt.Println(i18n.T("드라이브: %s:%s   설정됨: %v", s.Remote, s.Root, s.Configured))
	fmt.Println(i18n.T("자동 실행: %v   주기: %d분   일시 정지: %v", platform.AutostartEnabled(), s.IntervalMinutes, s.Paused))
	fmt.Println(i18n.T("실행 중: %v   데이터 폴더: %s", agent.ExistingURL() != "", store.Home()))
	if lock := store.ReadLock(); lock != nil {
		fmt.Println(i18n.T("지금 동기화 중: %s", lock.Folder))
	}
	var cfg config.Config
	if store.ReadJSON(store.CacheFile(), &cfg) != nil {
		fmt.Println(i18n.T("드라이브 설정을 아직 받지 않았습니다."))
		return 0
	}
	fmt.Println(i18n.T("폴더 (설정 revision %d):", cfg.Revision))
	for _, f := range cfg.Folders {
		mine := f.Paths[s.ClientID]
		if mine == "" {
			mine = i18n.T("(이 PC 에서는 동기화 안 함)")
		}
		fmt.Printf("- %s: %s  <->  %s:%s\n", f.Name, mine, s.Remote, f.DrivePath)
		if fst := st.Folders[f.ID]; fst != nil {
			line := "    " + i18n.T("마지막 성공 %s", orDash(fst.LastOK))
			if fst.LastError != "" {
				line += "   " + i18n.T("오류: %s", fst.LastError)
			}
			fmt.Println(line)
		}
	}
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func quitRunning() error {
	var info struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	if agent.ExistingURL() == "" || store.ReadJSON(store.UIFile(), &info) != nil {
		return errors.New(i18n.T("실행 중인 GodseonSync 가 없습니다"))
	}
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/quit", info.Port), strings.NewReader("{}"))
	req.Header.Set("X-GS-Token", info.Token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	// 동기화 중이면 안전하게 멈추느라 시간이 걸린다. 완전히 끝날 때까지 기다린다 (설치 프로그램이 파일을 바꿀 수 있게).
	for deadline := time.Now().Add(100 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if agent.ExistingURL() == "" {
			fmt.Println(i18n.T("GodseonSync 를 끝냈습니다."))
			return nil
		}
	}
	return errors.New(i18n.T("GodseonSync 가 100초 안에 끝나지 않았습니다"))
}

// runWindow 는 실행 중인 에이전트의 설정 화면을 전용 창으로 띄운다 (에이전트가 부른다).
// 에이전트가 "focus" 를 보내면 창을 앞으로 가져오고, "close" 를 보내거나 에이전트가 끝나면 창을 닫는다.
func runWindow() error {
	var info struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	url := agent.ExistingURL()
	if url == "" || store.ReadJSON(store.UIFile(), &info) != nil {
		return errors.New(i18n.T("실행 중인 GodseonSync 가 없습니다"))
	}
	go func() {
		client := &http.Client{Timeout: 40 * time.Second}
		fails := 0
		for {
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/window/poll", info.Port), nil)
			req.Header.Set("X-GS-Token", info.Token)
			resp, err := client.Do(req)
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				if fails++; fails >= 3 { // 에이전트가 끝났다
					window.Close()
					return
				}
				time.Sleep(time.Second)
				continue
			}
			fails = 0
			var body struct {
				Cmd string `json:"cmd"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			switch body.Cmd {
			case "focus":
				window.Focus()
			case "close":
				window.Close()
				return
			}
		}
	}()
	return window.Run(url)
}
