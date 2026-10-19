// Package web 은 설정 화면 파일(HTML·JS·CSS)을 실행 파일 안에 넣는다.
package web

import "embed"

// FS 는 설정 화면 파일이다.
//
//go:embed index.html i18n.js app.js style.css icon.svg
var FS embed.FS
