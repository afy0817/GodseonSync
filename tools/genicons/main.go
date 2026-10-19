// genicons 는 배포 패키지용 앱 아이콘을 만든다 (빌드 스크립트가 부른다).
//
//	go run ./tools/genicons -out build/icons
//
// 결과: AppIcon.iconset/*.png (맥 iconutil 용), app.ico (윈도우 실행 파일용), icon-256.png (리눅스)
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/afy0817/GodseonSync/internal/icon"
)

func main() {
	out := flag.String("out", "build/icons", "출력 폴더")
	flag.Parse()
	set := filepath.Join(*out, "AppIcon.iconset")
	must(os.MkdirAll(set, 0o755))
	// 맥 아이콘은 가장자리 여백을 둔다 (macOS 아이콘 격자: 1024 중 824)
	for _, s := range []int{16, 32, 128, 256, 512} {
		write(filepath.Join(set, fmt.Sprintf("icon_%dx%d.png", s, s)), icon.PNG(icon.Render(s, icon.AppStyle(0.1))))
		write(filepath.Join(set, fmt.Sprintf("icon_%dx%d@2x.png", s, s)), icon.PNG(icon.Render(s*2, icon.AppStyle(0.1))))
	}
	var imgs []*image.NRGBA
	for _, s := range []int{16, 24, 32, 48, 64, 256} {
		imgs = append(imgs, icon.Render(s, icon.AppStyle(0.02)))
	}
	write(filepath.Join(*out, "app.ico"), icon.ICO(imgs))
	write(filepath.Join(*out, "icon-256.png"), icon.PNG(icon.Render(256, icon.AppStyle(0.02))))
}

func write(p string, b []byte) { must(os.WriteFile(p, b, 0o644)) }

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
