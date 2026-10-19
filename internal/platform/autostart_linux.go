//go:build !darwin && !windows

package platform

import (
	"os"
	"path/filepath"
)

func autostartFile() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "godseonsync.desktop")
}

// AutostartEnabled 는 로그인할 때 자동으로 실행하도록 등록돼 있는지 알려 준다.
func AutostartEnabled() bool {
	_, err := os.Stat(autostartFile())
	return err == nil
}

// AutostartProblem 은 자동 실행을 등록하면 안 되는 상황을 설명한다 (없으면 빈 문자열).
func AutostartProblem() string { return "" }

// setAutostart 는 로그인 자동 실행(XDG autostart)을 켜거나 끈다. 켤 때 앱 메뉴 바로가기도 만든다. (SetAutostart 가 부른다)
func setAutostart(enabled bool) error {
	path := autostartFile()
	if !enabled {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe := Executable()
	entry := "[Desktop Entry]\nType=Application\nName=GodseonSync\nComment=GodseonSync\nTerminal=false\nCategories=Utility;\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(entry+"Exec=\""+exe+"\" agent --background\nX-GNOME-Autostart-enabled=true\n"), 0o644); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	menu := filepath.Join(home, ".local", "share", "applications", "godseonsync.desktop")
	if err := os.MkdirAll(filepath.Dir(menu), 0o755); err == nil {
		_ = os.WriteFile(menu, []byte(entry+"Exec=\""+exe+"\"\n"), 0o644)
	}
	return nil
}
