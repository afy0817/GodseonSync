package web

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	jsT      = regexp.MustCompile(`(?:^|[^\w$.])t\(('(?:[^'\\]|\\.)*'|"(?:[^"\\]|\\.)*")`)
	jsQuoted = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'`)
	enLine   = regexp.MustCompile(`(?m)^\s*("(?:[^"\\]|\\.)*"):\s*("(?:[^"\\]|\\.)*"),?\s*$`)
	hangul   = regexp.MustCompile(`[가-힣]`)
	holder   = regexp.MustCompile(`\{\w+\}|</?\w+`)
)

func lf(b []byte) []byte { return []byte(strings.ReplaceAll(string(b), "\r\n", "\n")) }

func jsString(lit string) string {
	body := lit[1 : len(lit)-1]
	r := strings.NewReplacer(`\'`, `'`, `\"`, `"`, `\\`, `\`)
	return r.Replace(body)
}

// 설정 화면의 t('…') 문구와 그릴 때 번역하는 표(MODES·SETTINGS_TABS)의 문구에는 모두 영어 번역이 있어야 하고,
// {자리}·HTML 태그가 원문과 같아야 한다.
func TestScreenTextTranslated(t *testing.T) {
	app, _ := os.ReadFile("app.js")
	i18n, _ := os.ReadFile("i18n.js")
	app, i18n = lf(app), lf(i18n) // 윈도우 체크아웃은 줄바꿈이 CRLF 일 수 있다
	en := map[string]string{}
	for _, m := range enLine.FindAllStringSubmatch(string(i18n), -1) {
		var k, v string
		if json.Unmarshal([]byte(m[1]), &k) == nil && json.Unmarshal([]byte(m[2]), &v) == nil {
			en[k] = v
		}
	}
	src := string(app)
	keys := map[string]bool{}
	for _, m := range jsT.FindAllStringSubmatch(src, -1) {
		keys[jsString(m[1])] = true
	}
	tables := ""
	for start, end := range map[string]string{"const MODES = {": "\n};\n", "const SETTINGS_TABS = ": ";\n"} {
		i := strings.Index(src, start)
		j := strings.Index(src[max(i, 0):], end)
		if i < 0 || j < 0 {
			t.Fatalf("app.js 에서 %q 표를 찾지 못함", start)
		}
		tables += src[i : i+j]
	}
	for _, m := range jsQuoted.FindAllStringSubmatch(tables, -1) {
		keys[strings.ReplaceAll(m[1], `\'`, `'`)] = true
	}
	n := 0
	for k := range keys {
		if !hangul.MatchString(k) {
			continue
		}
		n++
		e, ok := en[k]
		if !ok {
			t.Errorf("영어 번역 없음: %q", k)
			continue
		}
		a, b := holder.FindAllString(k, -1), holder.FindAllString(e, -1)
		if strings.Join(sorted(a), " ") != strings.Join(sorted(b), " ") {
			t.Errorf("자리 표시·태그가 다름: %q → %q", k, e)
		}
	}
	if n < 200 {
		t.Fatalf("문구를 찾지 못함 (%d개)", n)
	}
}

func sorted(s []string) []string {
	out := append([]string{}, s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
