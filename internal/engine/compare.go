package engine

import (
	"encoding/json"
	"sort"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/afy0817/GodseonSync/internal/rclone"
	"github.com/afy0817/GodseonSync/internal/store"
)

// 합치기 전에 양쪽(이 PC · 저장소) 파일 목록을 비교한다. 같으면 묻지 않고 합치고(바뀌는 것 없음),
// 다르면 차이를 store.ChoiceDiff 로 남겨 사용자가 보고 고르게 한다.

// modWindow 는 수정 시각이 이만큼 안쪽이면 같은 것으로 본다 (SFTP 는 초 단위, 드라이브는 밀리초 단위로 저장한다).
const modWindow = 2 * time.Second

// diffSamples 는 차이마다 화면에 보여 줄 파일 이름 수다.
const diffSamples = 5

type fileInfo struct {
	size int64
	mod  time.Time
}

// listFiles 는 spec 안에서 제외 규칙을 지난 파일 목록이다 (경로는 NFC — 맥의 자모 분리 이름도 같은 이름으로 본다).
// 폴더가 없으면 빈 목록.
func listFiles(rc *rclone.Client, spec, ffile string, flags []string) (map[string]fileInfo, error) {
	args := append([]string{"lsjson", "-R", "--files-only", "--no-mimetype", "--filter-from", ffile}, flags...)
	res, err := rc.Exec(30*time.Minute, nil, append(args, spec)...)
	if err != nil {
		return nil, err
	}
	if res.Code == rclone.CodeDirNotFound {
		return map[string]fileInfo{}, nil
	}
	if err := res.Err("lsjson"); err != nil {
		return nil, err
	}
	var items []struct {
		Path    string
		Size    int64
		ModTime time.Time
	}
	if err := json.Unmarshal([]byte(res.Stdout), &items); err != nil {
		return nil, err
	}
	out := make(map[string]fileInfo, len(items))
	for _, it := range items {
		out[norm.NFC.String(it.Path)] = fileInfo{it.Size, it.ModTime}
	}
	return out, nil
}

// compareFiles 는 두 목록의 차이다. 크기가 다르거나 수정 시각이 modWindow 넘게 다르면 '내용이 다른 파일'.
func compareFiles(local, remote map[string]fileInfo) store.ChoiceDiff {
	var d store.ChoiceDiff
	var onlyL, onlyR, diff []string
	for p, l := range local {
		r, ok := remote[p]
		switch {
		case !ok:
			onlyL = append(onlyL, p)
		case l.size != r.size || absDur(l.mod.Sub(r.mod)) > modWindow:
			diff = append(diff, p)
		}
	}
	for p := range remote {
		if _, ok := local[p]; !ok {
			onlyR = append(onlyR, p)
		}
	}
	d.OnlyLocal, d.OnlyDrive, d.Different = len(onlyL), len(onlyR), len(diff)
	d.OnlyLocalSamples, d.OnlyDriveSamples, d.DifferentSamples = samples(onlyL), samples(onlyR), samples(diff)
	return d
}

func samples(paths []string) []string {
	sort.Strings(paths)
	if len(paths) > diffSamples {
		paths = paths[:diffSamples]
	}
	return paths
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// sameFiles 는 차이가 없는지다.
func sameFiles(d store.ChoiceDiff) bool {
	return d.OnlyLocal == 0 && d.OnlyDrive == 0 && d.Different == 0
}
