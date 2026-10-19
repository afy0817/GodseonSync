// Package storage 는 동기화에 쓰는 저장소 방식(모드)을 정한다.
//
// 모드마다 rclone 연결 종류, 연결할 때 받는 값, 동기화할 때 붙이는 rclone 옵션이 다르다.
// 지금은 구글 드라이브(gdrive)와 SFTP(sftp). 모드를 더하려면
//  1. Mode 를 하나 만들어 modes 에 넣고 (이 패키지),
//  2. rclone 에 그 backend 를 넣고 (internal/rcloneembed),
//  3. 설정 화면의 입력 칸을 더한다 (web/app.js 의 MODES).
//
// 어느 모드든 공용 설정은 저장소의 GodseonSync/setting.json 이고, 이 PC 의 연결은 rclone.conf 의
// store.Settings.Remote 이름(기본 gdrive) 하나다. 모드는 그 연결의 rclone type 으로 알아낸다.
package storage

import "github.com/afy0817/GodseonSync/internal/rclone"

// Mode 는 저장소 방식 하나다.
type Mode struct {
	Key        string // 설정 화면·API 에서 쓰는 이름 (gdrive, sftp …)
	Label      string // 화면에 보이는 이름
	RcloneType string // rclone 연결 종류 (drive, sftp …)
	// Params 는 화면에서 받은 값을 검사하고 `rclone config create <이름> <종류>` 뒤에 붙일 key=value 로 바꾼다.
	// 오류는 그대로 사용자에게 보인다.
	Params func(in map[string]string) ([]string, error)
	// Info 는 rclone.conf 의 연결 항목에서 화면에 보여도 되는 값만 고른다 (토큰·비밀번호는 빼야 한다).
	Info func(conf map[string]string) map[string]string
	// SyncFlags 는 이 모드로 bisync 할 때 더 붙이는 rclone 옵션이다.
	SyncFlags []string
	// CreateFlags 는 `rclone config create` 에 더 붙이는 옵션이다 (SFTP: 비밀번호 가리기, 질문 없이).
	CreateFlags []string
	// Verify 면 연결을 만든 뒤 실제로 접속되는지 확인한다 (브라우저 로그인이 없는 모드는 만들기만 해서는 모른다).
	Verify bool
	// Where 는 화면·알림에 보여 줄 저장소 위치다 (구글: '구글 드라이브', SFTP: user@host:port). 비밀번호는 넣지 않는다.
	Where func(conf map[string]string) string
}

var modes = []*Mode{GoogleDrive, SFTP}

// Default 는 처음 설정 화면에서 먼저 고르는 모드다.
var Default = GoogleDrive

// All 은 모든 모드다 (화면의 선택지 순서).
func All() []*Mode { return modes }

// Get 은 이름으로 모드를 찾는다. 없으면 nil.
func Get(key string) *Mode {
	for _, m := range modes {
		if m.Key == key {
			return m
		}
	}
	return nil
}

// Of 는 rclone 연결의 모드다. 연결이 없거나 모르는 종류(테스트의 가짜 드라이브 alias 등)면 nil.
func Of(rc *rclone.Client, remote string) *Mode {
	conf, ok := rc.RemoteOptions(remote)
	if !ok {
		return nil
	}
	for _, m := range modes {
		if m.RcloneType == conf["type"] {
			return m
		}
	}
	return nil
}

// SyncFlags 는 그 연결로 동기화할 때 붙일 옵션이다 (모르는 모드면 없음).
func SyncFlags(rc *rclone.Client, remote string) []string {
	if m := Of(rc, remote); m != nil {
		return m.SyncFlags
	}
	return nil
}
