// Package buildinfo 는 빌드할 때 -ldflags "-X ..." 로 넣는 값을 담는다.
package buildinfo

// Version 은 릴리스 빌드에서 태그 이름으로 바뀐다.
var Version = "0.1.0-dev"

// 구글 OAuth 클라이언트. 비워 두면 rclone 에 들어 있는 공용 클라이언트를 쓴다.
// 직접 만든 클라이언트로 배포하려면 빌드할 때 넣는다 (README 의 "구글 OAuth 클라이언트" 참고).
var (
	GoogleClientID     = ""
	GoogleClientSecret = ""
)
