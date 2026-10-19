// Package platform 은 OS 별 차이(데이터 폴더, 브라우저 열기, 폴더 선택 창, 알림, 자동 실행)를 모은다.
package platform

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// OSKey 는 공용 설정에 기록하는 OS 이름이다 (mac / windows / linux).
var OSKey = map[string]string{"darwin": "mac", "windows": "windows"}[runtime.GOOS]

func init() {
	if OSKey == "" {
		OSKey = "linux"
	}
}

// AppDir 은 이 PC 전용 데이터(설정·상태·로그·구글 로그인 정보)를 두는 폴더다. 구글 드라이브에는 올라가지 않는다.
func AppDir() string {
	if d := os.Getenv("GODSEONSYNC_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "GodseonSync")
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "GodseonSync")
		}
		return filepath.Join(home, "AppData", "Local", "GodseonSync")
	default:
		if d := os.Getenv("XDG_STATE_HOME"); d != "" {
			return filepath.Join(d, "godseonsync")
		}
		return filepath.Join(home, ".local", "state", "godseonsync")
	}
}

// Executable 은 이 프로그램 실행 파일의 실제 경로다.
func Executable() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		return real
	}
	return exe
}

// Hostname 은 점 앞부분만 쓴 컴퓨터 이름이다.
func Hostname() string {
	h, _ := os.Hostname()
	h = strings.SplitN(h, ".", 2)[0]
	if h == "" {
		return "PC"
	}
	return h
}

var winEnv = regexp.MustCompile(`%([^%]+)%`)

// ExpandPath 는 ~ 와 환경 변수($HOME, %USERPROFILE%)를 풀어 쓴다.
func ExpandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	if runtime.GOOS == "windows" {
		p = winEnv.ReplaceAllStringFunc(p, func(m string) string {
			if v, ok := os.LookupEnv(m[1 : len(m)-1]); ok {
				return v
			}
			return m
		})
	}
	return filepath.Clean(os.ExpandEnv(p))
}

// OpenBrowser 는 기본 브라우저로 주소를 연다.
func OpenBrowser(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	HideWindow(c)
	return c.Start()
}

// LaunchBlocked 는 OS 보안 정책이 실행 파일을 막아 프로세스를 띄우지 못한 오류인지다.
// 윈도우 스마트 앱 컨트롤은 서명 없는 실행 파일을 평판으로 판단해서, 어제 되던 GodseonSync(rclone 을 띄우는 자기 자신)를 막기도 한다.
func LaunchBlocked(err error) bool { return launchBlocked(err) }

// SetupNotifications 는 알림을 눌렀을 때 할 일과 알림을 못 보낼 때 남길 곳을 정한다 (에이전트가 시작할 때 부른다).
// 맥은 이때 알림 권한을 묻는다 (처음 한 번). 윈도우는 알림에 보일 앱 이름·아이콘과 누르면 열 명령을 등록한다.
func SetupNotifications(onClick func(), onError func(string)) { setupNotify(onClick, onError) }

// Notify 는 OS 알림을 띄운다 (실패해도 무시). 맥·윈도우는 OS API 로 보내고, 못 보내면 아래 명령으로 보낸다.
func Notify(title, message string) {
	if nativeNotify(title, message) {
		return
	}
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("osascript", "-e", "display notification "+asString(message)+" with title "+asString(title))
	case "windows":
		script := "[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime];" +
			"$x = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);" +
			"$t = $x.GetElementsByTagName('text'); $t.Item(0).InnerText = " + psString(title) + "; $t.Item(1).InnerText = " + psString(message) + ";" +
			"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier(" + psString(psAppID) + ").Show([Windows.UI.Notifications.ToastNotification]::new($x))"
		c = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	default:
		if _, err := exec.LookPath("notify-send"); err != nil {
			return
		}
		c = exec.Command("notify-send", title, message)
	}
	HideWindow(c)
	_ = c.Start()
	go func() { _ = c.Wait() }()
}

// psAppID 는 GodseonSync 앱 ID 를 등록하지 못했을 때 빌려 쓰는 PowerShell 의 앱 ID 다 (등록되지 않은 ID 로는 알림이 안 뜬다).
const psAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// pickStart 는 폴더 선택 창을 처음 열 위치다: initial 이 있으면 그 폴더, 없으면 그 위 폴더, 둘 다 없으면 빈 문자열.
func pickStart(initial string) string {
	if initial == "" {
		return ""
	}
	p := ExpandPath(initial)
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	if st, err := os.Stat(filepath.Dir(p)); err == nil && st.IsDir() {
		return filepath.Dir(p)
	}
	return ""
}

// PickFolder 는 OS 기본 폴더 선택 창을 띄운다. 취소하면 빈 문자열.
// 윈도우 전용 설정 창은 이것 대신 창 프로세스가 PickFolderIn 으로 설정 창에 붙여 띄운다.
func PickFolder(initial, prompt string) (string, error) {
	start := pickStart(initial)
	switch runtime.GOOS {
	case "darwin":
		loc := ""
		if start != "" {
			loc = " default location (POSIX file " + asString(start) + ")"
		}
		out, err := exec.Command("osascript", "-e", "activate\nPOSIX path of (choose folder with prompt "+asString(prompt)+loc+")").Output()
		if err != nil {
			return "", nil // 취소
		}
		p := strings.TrimSpace(string(out))
		if p != "/" {
			p = strings.TrimRight(p, "/")
		}
		return p, nil
	case "windows":
		return pickFolderWindows(start, prompt)
	default:
		var c *exec.Cmd
		if _, err := exec.LookPath("zenity"); err == nil {
			args := []string{"--file-selection", "--directory", "--title=" + prompt}
			if start != "" {
				args = append(args, "--filename="+start+"/")
			}
			c = exec.Command("zenity", args...)
		} else if _, err := exec.LookPath("kdialog"); err == nil {
			if start == "" {
				start, _ = os.UserHomeDir()
			}
			c = exec.Command("kdialog", "--getexistingdirectory", start, "--title", prompt)
		} else {
			return "", nil
		}
		out, err := c.Output()
		if err != nil {
			return "", nil
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// asString 은 AppleScript 문자열 리터럴을 만든다 (\" 와 \\ 이스케이프는 JSON 과 같다).
func asString(s string) string {
	b, _ := json.Marshal(s)
	return strings.NewReplacer(`<`, "<", `>`, ">", `&`, "&").Replace(string(b))
}

func psString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
