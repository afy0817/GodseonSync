//go:build windows

package platform

import (
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValue = "GodseonSync"

// AutostartEnabled 는 로그인할 때 자동으로 실행하도록 등록돼 있는지 알려 준다.
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

// AutostartProblem 은 자동 실행을 등록하면 안 되는 상황을 설명한다 (없으면 빈 문자열).
func AutostartProblem() string { return "" }

// setAutostart 는 로그인 자동 실행을 켜거나 끈다. 켤 때 시작 메뉴 바로가기도 만든다. (SetAutostart 가 부른다)
func setAutostart(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		if err := k.DeleteValue(runValue); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	exe := Executable()
	if err := k.SetStringValue(runValue, `"`+exe+`" agent --background`); err != nil {
		return err
	}
	createStartMenuShortcut(exe)
	return nil
}

func createStartMenuShortcut(exe string) {
	script := "$p = Join-Path ([Environment]::GetFolderPath('Programs')) 'GodseonSync.lnk';" +
		"$s = (New-Object -ComObject WScript.Shell).CreateShortcut($p);" +
		"$s.TargetPath = " + psString(exe) + "; $s.WorkingDirectory = " + psString(filepath.Dir(exe)) + ";" +
		"$s.Description = 'GodseonSync'; $s.Save()"
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	HideWindow(c)
	_ = c.Run()
}
