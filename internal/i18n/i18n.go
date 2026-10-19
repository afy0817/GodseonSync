// Package i18n 은 사용자에게 보이는 문구를 이 PC 언어(한국어·영어)로 바꾼다.
//
// 한국어 문구를 그대로 키로 쓰고, 영어 번역은 en 표(en.go)에 둔다. 표에 없으면 한국어 그대로 나간다.
// 언어는 이 PC 설정(store.Settings.Language)을 읽을 때마다 SetLang 으로 맞춘다.
// 설정 화면(web/app.js)은 같은 방식의 자기 표를 따로 갖는다.
package i18n

import (
	"fmt"
	"sync/atomic"
)

// 지원 언어
const (
	Korean  = "ko"
	English = "en"
)

var lang atomic.Value

// SetLang 은 지금 언어를 바꾼다. 모르는 값이면 한국어.
func SetLang(l string) {
	if l != English {
		l = Korean
	}
	lang.Store(l)
}

// Lang 은 지금 언어다.
func Lang() string {
	if l, ok := lang.Load().(string); ok {
		return l
	}
	return Korean
}

// T 는 한국어 문구 ko 를 지금 언어로 바꾼다. args 가 있으면 fmt.Sprintf 처럼 채운다
// (번역에서도 %s·%d 의 순서와 개수는 같아야 한다).
func T(ko string, args ...any) string {
	s := ko
	if Lang() == English {
		if e, ok := en[ko]; ok {
			s = e
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Has 는 영어 번역이 있는지다 (테스트용).
func Has(ko string) bool {
	_, ok := en[ko]
	return ok
}

// Error 는 메시지를 쓸 때마다 지금 언어로 바꾸는 오류다 (errors.Is 로 비교하는 패키지 변수용).
func Error(ko string) error { return &transError{ko} }

type transError struct{ ko string }

func (e *transError) Error() string { return T(e.ko) }
