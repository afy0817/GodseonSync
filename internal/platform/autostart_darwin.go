//go:build darwin

package platform

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/afy0817/GodseonSync/internal/i18n"
)

// LaunchAgentLabel 은 로그인할 때 GodseonSync 를 띄우는 launchd 작업 이름이다.
const LaunchAgentLabel = "io.github.afy0817.godseonsync"

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", LaunchAgentLabel+".plist")
}

// AutostartEnabled 는 로그인할 때 자동으로 실행하도록 등록돼 있는지 알려 준다.
func AutostartEnabled() bool {
	_, err := os.Stat(launchAgentPath())
	return err == nil
}

// AutostartProblem 은 자동 실행을 등록하면 안 되는 상황을 설명한다 (없으면 빈 문자열).
func AutostartProblem() string {
	exe := Executable()
	if strings.Contains(exe, "/AppTranslocation/") || strings.HasPrefix(exe, "/Volumes/") {
		return i18n.T("GodseonSync.app 을 응용 프로그램(Applications) 폴더로 옮긴 뒤 다시 실행하세요. 지금 위치에서는 자동 실행을 등록할 수 없습니다.")
	}
	return ""
}

// setAutostart 는 로그인 자동 실행을 켜거나 끈다. (SetAutostart 가 부른다)
func setAutostart(enabled bool) error {
	path := launchAgentPath()
	if !enabled {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if p := AutostartProblem(); p != "" {
		return fmt.Errorf("%s", p)
	}
	logPath := filepath.Join(AppDir(), "launchd.log")
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + LaunchAgentLabel + `</string>
	<key>ProgramArguments</key>
	<array><string>` + xmlEscape(Executable()) + `</string><string>agent</string><string>--background</string></array>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
	<key>LimitLoadToSessionType</key><string>Aqua</string>
	<key>ProcessType</key><string>Interactive</string>
	<key>StandardOutPath</key><string>` + xmlEscape(logPath) + `</string>
	<key>StandardErrorPath</key><string>` + xmlEscape(logPath) + `</string>
</dict>
</plist>
`)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return err
	}
	// 지금 떠 있는 GodseonSync 가 있으므로 launchd 에 바로 올리지는 않는다 (다음 로그인부터 실행).
	// 로그인 때 launchd 가 띄운 것이 지금 프로세스일 수 있으니 bootout 도 하지 않는다.
	return nil
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
