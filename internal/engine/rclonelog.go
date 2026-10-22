package engine

import (
	"regexp"
	"strings"

	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/store"
)

// rcloneLog 는 rclone 출력을 사람이 읽기 쉬운 줄로 바꿔 폴더 로그에 남기고, 오간 파일 수를 센다.
//   - 바뀐 게 없어도 매번 똑같이 나오는 줄(비교 설정, 잠금 갱신, 목록 만들기 …)은 남기지 않는다.
//   - 파일 줄에는 방향을 붙인다: ↑ 올림(이 PC → 저장소), ↓ 받음(저장소 → 이 PC), − 지움.
//   - 오류·경고와 모르는 줄은 rclone 문구 그대로 남긴다.
//
// bisync 에서 Path1 은 이 PC, Path2 는 저장소다. 방향은 'Do queued copies to - Path2' 같은 줄로 알아내고,
// 실제로 옮긴 파일은 그 뒤에 나오는 '<경로>: Copied (new)' 같은 줄로 센다.
type rcloneLog struct {
	lg     store.FolderLog
	store  string            // 저장소를 부르는 말 (드라이브, SFTP)
	roots  []string          // 파일 경로를 폴더 기준으로 줄일 때 지우는 앞부분 (이 PC 경로, 저장소 경로)
	dry    bool              // 미리보기
	quiet  bool              // 파일 줄은 남기지 않고 세기만 한다 (덮어쓰기 전 백업)
	dir    int               // 지금 복사 방향
	inJSON bool              // 'Comparison Settings' 뒤에 오는 여러 줄 JSON 을 건너뛰는 중
	both   string            // 양쪽에서 바뀐 것 같은 파일 — 정말 충돌인지는 뒤 줄로 안다 (내용이 같으면 rclone 이 그냥 넘어간다)
	dirs   map[string]bool   // 이미 남긴 새 폴더
	kinds  map[string]string // bisync 가 먼저 알려 준 파일별 바뀐 종류 (File is new → new) — 서버 쪽 복사는 새 파일인지 안 알려 준다
	n      struct{ up, down, delRemote, delLocal, other, conflicts, copied int }
	bytes  string // 마지막 통계 줄의 전송량
}

const (
	toRemote = 1 // 이 PC → 저장소
	toLocal  = 2 // 저장소 → 이 PC
)

func newRcloneLog(lg store.FolderLog, storeWord string, dry bool, roots ...string) *rcloneLog {
	return &rcloneLog{lg: lg, store: storeWord, dry: dry, roots: roots, dirs: map[string]bool{}, kinds: map[string]string{}}
}

var (
	rcLevel = regexp.MustCompile(`^(DEBUG|INFO|NOTICE|WARNING|ERROR|CRITICAL|ALERT|EMERGENCY)\s*:\s?(.*)$`)
	// bisync 의 '- <Path1|Path2|WARNING> <할 일> - <파일>' (rclone cmd/bisync/log.go)
	rcIndent = regexp.MustCompile(`^-\s+(?:(Path[12]|WARNING)\s+)?(.+?)\s+- (.*)$`)
	// 파일 하나를 옮기고 지운 줄 (rclone fs/operations)
	rcFileOp    = regexp.MustCompile(`^(.+): ((?:Multi-thread )?Copied \(([^)]*)\)(?: to: .*)?|Deleted|Updated modification time in destination|Making directory|Made directory with .*|Removing directory|Skipped (\w+(?: \w+)*) as --dry-run is set.*)$`)
	rcFileQuiet = regexp.MustCompile(`^.+: (Set directory modification time.*|Updated directory metadata|Moved \(server-side\).*|renamed from: .*|` +
		`Path[12] is (newer|older|larger|smaller)\. .*|The winner is: .*|Winner cannot be determined.*|` +
		`\d+ (differences found|matching files|hashes could not be checked|files missing|errors while checking))$`)
	// '--stats-one-line' 통계: '24.622 KiB / 24.622 KiB, 100%, 24.619 KiB/s, ETA 0s'
	rcStats = regexp.MustCompile(`^(\d[\d.]* ?\S*B) / (\d[\d.]* ?\S*B), (-|\d+%), `)
	// 바뀐 게 없어도 매번 나오는 줄, 아래 파일 줄과 겹치는 줄
	rcQuiet = regexp.MustCompile(`^(Setting --ignore-listing-checksum|lock file renewed|Config file .* not found - using defaults|Synching Path1 |Using filters file |` +
		`(Skipped )?[Ss]toring filters file hash|Building Path1 and Path2 listings|Path[12] checking for diffs|No changes found|` +
		`Applying changes|(Resync )?[Uu]pdating listings|Validating (backup )?listings for |Bisync successful|Copying Path2 files to Path1|` +
		`Checking access health|Path[12]:\s+\d+ changes:|\(Modified:|There was nothing to transfer|Not checking for common hash|` +
		`Checking potential conflicts|Finished checking the potential conflicts|Files are equal|This is a directory, not a file|` +
		`Winner cannot be determined|Path[12] is (newer|older|larger|smaller)\.|Checks finished, now starting transfers|` +
		`Running all checks before starting transfers|Transfer session |Removing empty directories|` +
		`Bisync critical error: <nil>$|Failed to bisync with \d+ errors?: )`) // 실패 때 원인 없이 덧붙는 줄 (원인은 앞 오류 줄)
)

// line 은 rclone 출력 한 줄을 읽는다 (시각은 이미 지운 줄).
func (p *rcloneLog) line(raw string) {
	level, msg := "", strings.TrimSpace(raw)
	if m := rcLevel.FindStringSubmatch(msg); m != nil {
		level, msg = m[1], strings.TrimSpace(m[2])
	}
	if p.inJSON {
		if level == "" {
			p.inJSON = msg != "}"
			return
		}
		p.inJSON = false
	}
	switch {
	case msg == "":
		return
	case strings.HasPrefix(msg, "Bisyncing with Comparison Settings"):
		p.inJSON = true
		return
	case rcQuiet.MatchString(msg), rcFileQuiet.MatchString(msg):
		return
	}
	if m := rcStats.FindStringSubmatch(msg); m != nil {
		p.bytes = m[1]
		if m[3] != "-" && m[3] != "100%" { // 오래 걸리는 동기화는 1분마다 얼마나 했는지 남긴다
			p.lg.Log("  … 진행 중: %s / %s (%s)", m[1], m[2], m[3])
		}
		return
	}
	if m := rcIndent.FindStringSubmatch(msg); m != nil && p.indent(m[1], m[2], m[3]) {
		return
	}
	if m := rcFileOp.FindStringSubmatch(msg); m != nil {
		p.fileOp(m[1], m[2], m[3], m[4])
		return
	}
	switch {
	case level == "ERROR" || level == "CRITICAL" || level == "ALERT" || level == "EMERGENCY":
		p.lg.Log("  ✗ %s", msg)
	case level == "WARNING" || strings.HasPrefix(msg, "WARNING"):
		p.lg.Log("  ⚠ %s", msg)
	default:
		p.lg.Log("  %s", msg)
	}
}

// indent 는 bisync 의 '- 태그 할 일 - 파일' 줄이다. 남길 필요가 없거나 남겼으면 참.
func (p *rcloneLog) indent(tag, what, file string) bool {
	switch {
	case what == "Do queued copies to" || what == "Resync is copying files to":
		p.dir = map[string]int{"Path1": toLocal, "Path2": toRemote}[file]
	case what == "File is new":
		p.kinds[file] = "new"
	case strings.HasPrefix(what, "File changed"):
		p.kinds[file] = "changed"
	case strings.HasPrefix(what, "Queue "), strings.HasPrefix(what, "File "):
		// 할 일 목록과 지워진 것 찾기 — 실제로 옮긴 줄(Copied·Deleted)로 남긴다
	case what == "New or changed in both paths":
		p.both = p.rel(file)
	case strings.HasPrefix(what, "Not renaming"), strings.HasPrefix(what, "Deleting Path"):
		p.conflict()
	case strings.HasPrefix(what, "Renaming Path"):
		p.conflict()
		side := p.store
		if strings.HasPrefix(what, "Renaming Path1") {
			side = i18n.T("이 PC")
		}
		p.lg.Log("    %s 쪽 파일을 %s 로 이름 바꿔 남김", side, p.rel(file))
	default:
		return false
	}
	return true
}

// conflict 는 양쪽에서 바뀐 파일이 정말 충돌이었다고 남긴다 (rclone 이 이긴 쪽·진 쪽을 정한 줄이 왔다).
func (p *rcloneLog) conflict() {
	if p.both == "" {
		return
	}
	p.n.conflicts++
	p.lg.Log("  ⚠ 양쪽에서 바뀜: %s — 더 최신 파일을 남기고, 다른 쪽은 이름 뒤에 번호를 붙여 남깁니다", p.both)
	p.both = ""
}

func (p *rcloneLog) fileOp(path, action, copied, skipped string) {
	switch {
	case skipped != "": // 미리보기: 바꾸지 않고 건너뜀 (폴더 시각·이름 바꾸기 같은 건 위의 줄들로 충분하다)
		switch skipped {
		case "copy", "upload from pipe", "update modification time", "make directory":
			p.file(path, i18n.T(" (미리보기)"), false)
		case "delete", "remove directory":
			p.file(path, i18n.T(" (미리보기)"), true)
		}
	case copied == "new" || strings.HasSuffix(copied, ", new"):
		p.file(path, i18n.T(" (새 파일)"), false)
	case copied == "server-side copy": // 같은 저장소 안 복사 (가짜 드라이브·백업) — 앞서 알려 준 종류로
		p.file(path, map[string]string{"new": i18n.T(" (새 파일)"), "changed": i18n.T(" (수정)")}[p.kinds[path]], false)
	case copied != "":
		p.file(path, i18n.T(" (수정)"), false)
	case action == "Deleted":
		p.file(path, "", true)
	case action == "Updated modification time in destination":
		p.file(path, i18n.T(" (수정 시각)"), false)
	case action == "Removing directory":
		p.file(path, i18n.T(" (폴더)"), true)
	default: // 새 폴더 ('Making directory' 와 'Made directory …' 가 같이 나오기도 한다)
		if p.dirs[path] {
			return
		}
		p.dirs[path] = true
		p.file(path, i18n.T(" (새 폴더)"), false)
	}
}

func (p *rcloneLog) file(path, kind string, del bool) {
	if p.quiet {
		p.n.copied++
		return
	}
	switch {
	case del && p.dir == toRemote:
		p.n.delRemote++
		p.lg.Log("  − %s에서 지움%s: %s", p.store, kind, path)
	case del && p.dir == toLocal:
		p.n.delLocal++
		p.lg.Log("  − 이 PC 에서 지움%s: %s", kind, path)
	case p.dir == toRemote:
		p.n.up++
		p.lg.Log("  ↑ 올림%s: %s", kind, path)
	case p.dir == toLocal:
		p.n.down++
		p.lg.Log("  ↓ 받음%s: %s", kind, path)
	default:
		p.n.other++
		if del {
			p.lg.Log("  − 지움%s: %s", kind, path)
		} else {
			p.lg.Log("  %s%s", path, kind)
		}
	}
}

// rel 은 bisync 가 붙인 연결 이름·폴더 경로('local{abc}:/Users/…/폴더/')를 지운 폴더 안 경로다.
func (p *rcloneLog) rel(file string) string {
	for _, r := range p.roots {
		if r = strings.TrimRight(r, `/\`); r == "" {
			continue
		}
		if i := strings.Index(file, r); i >= 0 {
			if rest := strings.TrimLeft(file[i+len(r):], `/\`); rest != "" {
				return rest
			}
		}
	}
	return file
}

// summary 는 오간 파일 수 한 줄이다 (예: '올림 2개, 받음 1개, 24.6 KiB').
func (p *rcloneLog) summary() string {
	var parts []string
	add := func(n int, s string) {
		if n > 0 {
			parts = append(parts, s)
		}
	}
	add(p.n.up, i18n.T("올림 %d개", p.n.up))
	add(p.n.down, i18n.T("받음 %d개", p.n.down))
	add(p.n.delRemote, i18n.T("%s에서 %d개 지움", p.store, p.n.delRemote))
	add(p.n.delLocal, i18n.T("이 PC 에서 %d개 지움", p.n.delLocal))
	add(p.n.other, i18n.T("기타 %d개", p.n.other))
	add(p.n.conflicts, i18n.T("충돌 %d개", p.n.conflicts))
	if len(parts) == 0 {
		return i18n.T("바뀐 파일 없음")
	}
	if p.bytes != "" && !strings.HasPrefix(p.bytes, "0 ") && !p.dry {
		parts = append(parts, p.bytes)
	}
	return strings.Join(parts, ", ")
}
