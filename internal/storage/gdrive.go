package storage

import (
	"errors"
	"strings"

	"github.com/afy0817/GodseonSync/internal/buildinfo"
	"github.com/afy0817/GodseonSync/internal/i18n"
)

// GoogleDrive 는 구글 드라이브 모드다. 브라우저에서 구글 계정으로 로그인한다(OAuth).
//
// 받는 값: scope(drive.file 기본 / drive), client_id, client_secret.
// 클라이언트 ID 를 비우면 빌드에 넣은 값, 그것도 없으면 rclone 공용 클라이언트를 쓴다.
// 공용 클라이언트는 전 세계가 같이 써서 할당량에 자주 걸린다 (docs/lessons.md).
//
// 범위: drive.file(이 앱이 만든 파일만)은 구글이 민감하지 않은 범위로 보아, 앱 검증 없이도 '확인되지 않은 앱'
// 경고와 사용자 100명 한도가 없다. 대신 같은 클라이언트(구글 클라우드 프로젝트)로 만든 파일만 보인다.
// drive(드라이브 전체)는 제한된 범위라 검증 전에는 경고·한도가 있다.
var GoogleDrive = &Mode{
	Key:        "gdrive",
	Label:      "구글 드라이브",
	RcloneType: "drive",
	Params:     googleParams,
	Info: func(conf map[string]string) map[string]string {
		scope := conf["scope"]
		if scope == "" {
			scope = "drive"
		}
		info := map[string]string{"scope": scope, "client_id": conf["client_id"]}
		if conf["client_id"] != "" && conf["client_id"] == buildinfo.GoogleClientID {
			info["builtin"] = "1" // 앱(빌드)에 들어 있는 클라이언트
		}
		return info
	},
	SyncFlags: []string{"--drive-skip-gdocs"},
	Where:     func(map[string]string) string { return i18n.T("구글 드라이브") },
}

func googleParams(in map[string]string) ([]string, error) {
	scope := in["scope"]
	if scope != "drive" {
		scope = "drive.file"
	}
	id, secret := strings.TrimSpace(in["client_id"]), strings.TrimSpace(in["client_secret"])
	switch {
	case id == "" && secret != "":
		return nil, errors.New(i18n.T("클라이언트 ID 도 입력하세요."))
	case id != "" && secret == "":
		return nil, errors.New(i18n.T("클라이언트 보안 비밀도 입력하세요."))
	case id != "" && !strings.HasSuffix(id, ".apps.googleusercontent.com"):
		return nil, errors.New(i18n.T("클라이언트 ID 는 '….apps.googleusercontent.com' 으로 끝나야 합니다. 구글 클라우드 콘솔의 '사용자 인증 정보' 에서 데스크톱 앱 클라이언트의 ID 를 복사하세요."))
	case strings.ContainsAny(id+secret, " \t\r\n"):
		return nil, errors.New(i18n.T("클라이언트 ID·보안 비밀에 빈칸이 들어 있습니다."))
	}
	if id == "" {
		id, secret = buildinfo.GoogleClientID, buildinfo.GoogleClientSecret
	}
	params := []string{"scope=" + scope}
	if id != "" {
		params = append(params, "client_id="+id, "client_secret="+secret)
	}
	return params, nil
}

// FileScopeOptions 는 '이 앱이 만든 파일만'(drive.file) 범위로 옮길 때 쓸 연결 값이다.
// 클라이언트는 화면에서 넣은 것 → 빌드에 들어 있는 것 → 지금 연결(current, rclone.conf)의 것 순서로 쓴다.
// 보안 비밀은 화면에 보내지 않으므로, 지금 클라이언트를 그대로 쓸 때는 rclone.conf 에서 가져온다.
func FileScopeOptions(in, current map[string]string) map[string]string {
	out := map[string]string{"scope": "drive.file", "client_id": strings.TrimSpace(in["client_id"]), "client_secret": strings.TrimSpace(in["client_secret"])}
	if out["client_id"] == "" && buildinfo.GoogleClientID == "" && current["client_id"] != "" {
		out["client_id"], out["client_secret"] = current["client_id"], current["client_secret"]
	}
	return out
}
