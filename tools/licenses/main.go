// licenses 는 빌드한 실행 파일에 들어간 Go 모듈들의 라이선스 문서를 모아 한 파일로 만든다.
// 배포 파일(dmg·설치 파일·zip)에 같이 넣는다 (rclone 등 MIT 라이선스는 고지문을 함께 배포해야 한다).
//
//	go run ./tools/licenses <실행 파일> <출력 파일>
package main

import (
	"debug/buildinfo"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "사용법: go run ./tools/licenses <실행 파일> <출력 파일>")
		os.Exit(2)
	}
	info, err := buildinfo.ReadFile(os.Args[1])
	check(err)
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	check(err)
	cache := strings.TrimSpace(string(out))

	var b strings.Builder
	b.WriteString("GodseonSync 에 들어 있는 오픈 소스 구성 요소의 라이선스\n")
	b.WriteString("Third-party licenses of open source components included in GodseonSync\n\n")
	deps := info.Deps
	sort.Slice(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })
	var missing []string
	for _, d := range deps {
		m := d
		if d.Replace != nil {
			m = d.Replace
		}
		dir := filepath.Join(cache, escape(m.Path)+"@"+m.Version)
		files := licenseFiles(dir)
		if len(files) == 0 {
			missing = append(missing, m.Path+" "+m.Version)
			continue
		}
		b.WriteString(strings.Repeat("=", 78) + "\n")
		fmt.Fprintf(&b, "%s %s\n", m.Path, m.Version)
		b.WriteString(strings.Repeat("=", 78) + "\n")
		for _, f := range files {
			text, err := os.ReadFile(f)
			check(err)
			b.Write(text)
			b.WriteString("\n")
		}
	}
	if len(missing) > 0 {
		b.WriteString(strings.Repeat("=", 78) + "\n")
		b.WriteString("라이선스 파일을 모듈 안에서 찾지 못한 구성 요소 (각 저장소에서 확인하세요):\n")
		for _, m := range missing {
			b.WriteString("  " + m + "\n")
		}
	}
	check(os.WriteFile(os.Args[2], []byte(b.String()), 0o644))
	fmt.Printf("%d 개 모듈, 라이선스 못 찾음 %d 개 → %s\n", len(deps), len(missing), os.Args[2])
}

// escape 는 모듈 캐시 경로 규칙대로 대문자를 '!소문자' 로 바꾼다.
func escape(path string) string {
	var b strings.Builder
	for _, r := range path {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune('!')
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func licenseFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := strings.ToUpper(e.Name())
		if !e.IsDir() && (strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "LICENCE") ||
			strings.HasPrefix(n, "COPYING") || strings.HasPrefix(n, "NOTICE")) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
