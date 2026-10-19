package storage

import (
	"errors"
	"strconv"
	"strings"

	"github.com/afy0817/GodseonSync/internal/i18n"
)

// SFTP 는 SFTP 서버 모드다 (NAS·리눅스 서버 등). 로그인한 사용자의 홈 폴더 아래 GodseonSync 폴더를 쓴다.
//
// 받는 값: host, port(기본 22), user, auth(password / key), pass, key_file, key_file_pass,
// path(기본 위치 — 비우면 홈 폴더. chroot 로 홈 폴더 맨 위에 쓸 수 없는 서버는 upload 같은 하위 폴더나 /volume1/sync).
// path 는 rclone 이 아니라 GodseonSync 가 쓰는 값이라 gs_path 로 넘긴다 (서버가 rclone.conf 의 연결 칸에 따로 쓴다).
// 비밀번호·키 암호는 rclone 이 가려서(obscure) rclone.conf 에 저장한다 (--obscure). 화면에는 돌려주지 않는다.
// 호스트 키는 확인하지 않는다 (rclone sftp 기본값 — known_hosts 를 쓰려면 rclone.conf 에 known_hosts_file).
var SFTP = &Mode{
	Key:         "sftp",
	Label:       "SFTP",
	RcloneType:  "sftp",
	Params:      sftpParams,
	CreateFlags: []string{"--obscure", "--non-interactive"},
	Verify:      true,
	Info: func(conf map[string]string) map[string]string {
		auth := "password"
		if conf["key_file"] != "" || conf["key_pem"] != "" {
			auth = "key"
		}
		port := conf["port"]
		if port == "" {
			port = "22"
		}
		return map[string]string{"host": conf["host"], "port": port, "user": conf["user"], "auth": auth, "path": conf["gs_path"]}
	},
	Where: func(conf map[string]string) string {
		port := conf["port"]
		if port == "" {
			port = "22"
		}
		where := conf["user"] + "@" + conf["host"] + ":" + port
		if p := strings.TrimLeft(conf["gs_path"], "/"); p != "" {
			where += "/" + p
		}
		return where
	},
}

func sftpParams(in map[string]string) ([]string, error) {
	host, user := strings.TrimSpace(in["host"]), strings.TrimSpace(in["user"])
	port := strings.TrimSpace(in["port"])
	if port == "" {
		port = "22"
	}
	switch n, err := strconv.Atoi(port); {
	case host == "":
		return nil, errors.New(i18n.T("호스트를 입력하세요."))
	case strings.ContainsAny(host, " /\\:@"):
		return nil, errors.New(i18n.T("호스트는 이름이나 IP 주소만 입력하세요 (예: nas.local, 192.168.0.10)."))
	case err != nil || n < 1 || n > 65535:
		return nil, errors.New(i18n.T("포트는 1~65535 사이 숫자여야 합니다."))
	case user == "":
		return nil, errors.New(i18n.T("사용자 이름을 입력하세요."))
	}
	params := []string{"host=" + host, "port=" + port, "user=" + user}
	if p := strings.TrimRight(strings.TrimSpace(in["path"]), "/"); p != "" {
		if strings.Contains("/"+p+"/", "/../") || strings.ContainsAny(p, "\\:") {
			return nil, errors.New(i18n.T("폴더 위치에는 '..', '\\', ':' 를 쓸 수 없습니다."))
		}
		params = append(params, "gs_path="+p)
	}
	if in["auth"] == "key" {
		key := strings.TrimSpace(in["key_file"])
		if key == "" {
			return nil, errors.New(i18n.T("키 파일 경로를 입력하세요."))
		}
		params = append(params, "key_file="+key)
		if p := in["key_file_pass"]; p != "" {
			params = append(params, "key_file_pass="+p)
		}
		return params, nil
	}
	if in["pass"] == "" {
		return nil, errors.New(i18n.T("비밀번호를 입력하세요."))
	}
	return append(params, "pass="+in["pass"]), nil
}
