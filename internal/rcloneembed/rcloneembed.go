// Package rcloneembed 는 rclone 을 이 실행 파일 안에 넣는다.
//
// GodseonSync 는 rclone 이 필요한 작업마다 자기 자신을 `GodseonSync rclone <인자>` 로 다시 실행한다.
// 그래서 rclone 을 따로 설치할 필요가 없고, rclone 명령줄과 똑같이 동작한다.
// 필요한 백엔드(구글 드라이브·로컬)와 명령만 넣어서 크기를 줄였다.
package rcloneembed

import (
	"os"

	_ "github.com/rclone/rclone/backend/alias" // 테스트에서 가짜 드라이브로 쓴다
	_ "github.com/rclone/rclone/backend/drive"
	_ "github.com/rclone/rclone/backend/local"
	_ "github.com/rclone/rclone/backend/sftp" // SFTP 모드
	"github.com/rclone/rclone/cmd"
	_ "github.com/rclone/rclone/cmd/bisync"
	_ "github.com/rclone/rclone/cmd/cat"
	_ "github.com/rclone/rclone/cmd/config"
	_ "github.com/rclone/rclone/cmd/copy"
	_ "github.com/rclone/rclone/cmd/dedupe"
	_ "github.com/rclone/rclone/cmd/delete"
	_ "github.com/rclone/rclone/cmd/deletefile"
	_ "github.com/rclone/rclone/cmd/listremotes"
	_ "github.com/rclone/rclone/cmd/lsf"
	_ "github.com/rclone/rclone/cmd/lsjson"
	_ "github.com/rclone/rclone/cmd/mkdir"
	_ "github.com/rclone/rclone/cmd/moveto"
	_ "github.com/rclone/rclone/cmd/purge"
	_ "github.com/rclone/rclone/cmd/rcat"
	_ "github.com/rclone/rclone/cmd/sync"
	_ "github.com/rclone/rclone/cmd/version"
)

// Main 은 rclone 명령줄처럼 동작하고 끝나면 프로세스를 종료한다.
func Main(args []string) {
	os.Args = append([]string{"rclone"}, args...)
	cmd.Main()
	os.Exit(0)
}
