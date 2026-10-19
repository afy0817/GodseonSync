//go:build !windows

package platform

// pickFolderWindows 는 윈도우에서만 쓴다.
func pickFolderWindows(start, prompt string) (string, error) { return "", nil }
