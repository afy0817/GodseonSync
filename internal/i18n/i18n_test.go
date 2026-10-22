package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestTranslate(t *testing.T) {
	t.Cleanup(func() { SetLang(Korean) })
	SetLang(English)
	if T("[%s] 동기화 실패", "문서") != "[문서] Sync failed" || T("표에 없는 말") != "표에 없는 말" {
		t.Fatalf("영어: %q / %q", T("[%s] 동기화 실패", "문서"), T("표에 없는 말"))
	}
	SetLang("fr")
	if Lang() != Korean || T("[%s] 동기화 실패", "x") != "[x] 동기화 실패" {
		t.Fatal("모르는 언어는 한국어")
	}
	e := Error("다른 동기화가 실행 중입니다")
	SetLang(English)
	if e.Error() != "Another sync is running" {
		t.Fatalf("오류는 쓸 때 번역: %q", e.Error())
	}
}

var (
	msgCall = regexp.MustCompile(`(?:i18n\.T|i18n\.Error|\.Log|\.Both|[^\w.]Log|[^\w.]fail\(http\.\w+,)\s*\(?\s*("(?:[^"\\]|\\.)*")`)
	hangul  = regexp.MustCompile(`[가-힣]`)
	verb    = regexp.MustCompile(`%(?:\[\d+\])?[-+# 0]*\d*(?:\.\d+)?[a-zA-Z]`)
)

// 사용자에게 보이는 문구(i18n.T·i18n.Error·store.Log·폴더 로그 Log·Both·서버 fail)에는 모두 영어 번역이 있고, 형식 지정자가 같아야 한다.
func TestAllMessagesTranslated(t *testing.T) {
	root := filepath.Join("..")
	n := 0
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.Contains(p, "/i18n/") {
			return err
		}
		b, _ := os.ReadFile(p)
		for _, m := range msgCall.FindAllStringSubmatch(string(b), -1) {
			k, err := strconv.Unquote(m[1])
			if err != nil || !hangul.MatchString(k) {
				continue
			}
			n++
			e, ok := en[k]
			if !ok {
				t.Errorf("%s: 영어 번역 없음: %q", p, k)
				continue
			}
			if strings.Join(verb.FindAllString(k, -1), " ") != strings.Join(verb.FindAllString(e, -1), " ") {
				t.Errorf("%s: 형식 지정자가 다름: %q → %q", p, k, e)
			}
		}
		return nil
	})
	if err != nil || n < 100 {
		t.Fatalf("문구를 찾지 못함 (%d개, %v)", n, err)
	}
}
