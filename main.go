// GodseonSync — 여러 PC 의 폴더를 구글 드라이브를 거쳐 구조 그대로 양방향 동기화한다.
package main

import (
	"os"

	"github.com/afy0817/GodseonSync/internal/app"
	"github.com/afy0817/GodseonSync/internal/rcloneembed"
)

func main() {
	// 동기화 작업은 자기 자신을 `GodseonSync rclone ...` 으로 다시 실행해서 내장 rclone 에 맡긴다
	if len(os.Args) > 1 && os.Args[1] == "rclone" {
		rcloneembed.Main(os.Args[2:])
	}
	os.Exit(app.Main(os.Args[1:]))
}
