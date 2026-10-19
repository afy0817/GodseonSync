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

// SetupNotifications 는 알림을 눌렀을 때 할 일과 알림을 못 보낼 때 남길 곳을 정한다 (에이전트가 시작할 때 부른다).
// 맥은 이때 알림 권한을 묻는다 (처음 한 번).
func SetupNotifications(onClick func(), onError func(string)) { setupNotify(onClick, onError) }

// Notify 는 OS 알림을 띄운다 (실패해도 무시).
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

// psAppID 는 윈도우 알림을 띄울 때 쓰는 앱 ID 다. 등록되지 않은 ID 로는 알림이 안 떠서 PowerShell 의 ID 를 빌린다.
const psAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// PickFolder 는 OS 기본 폴더 선택 창을 띄운다. 취소하면 빈 문자열.
func PickFolder(initial, prompt string) (string, error) {
	start := ""
	if initial != "" {
		p := ExpandPath(initial)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			start = p
		} else if st, err := os.Stat(filepath.Dir(p)); err == nil && st.IsDir() {
			start = filepath.Dir(p)
		}
	}
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
		script := "[Console]::OutputEncoding = [Text.Encoding]::UTF8\n" +
			"Add-Type -AssemblyName System.Windows.Forms\n" +
			"$d = New-Object System.Windows.Forms.FolderBrowserDialog\n" +
			"$d.Description = " + psString(prompt) + "\n" +
			"$d.ShowNewFolderButton = $true\n"
		if start != "" {
			script += "$d.SelectedPath = " + psString(start) + "\n"
		}
		script += "$o = New-Object System.Windows.Forms.Form -Property @{TopMost = $true}\n" +
			"if ($d.ShowDialog($o) -eq 'OK') { $d.SelectedPath }\n"
		c := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script)
		HideWindow(c)
		out, err := c.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
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
