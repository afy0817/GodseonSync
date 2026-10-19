package store

import (
	"strings"
	"sync"
)

// 알림 센터: 동기화 실패·결정 필요·덮어쓰기·옮기기 같은 일을 이 PC 의 notices.json 에 남긴다.
// 설정 창을 닫아 둔 사이에 백그라운드에서 생긴 일도 남고, 화면에서 읽음 표시·삭제를 한다.

// Notice 는 알림 하나다.
type Notice struct {
	ID    string `json:"id"`
	Time  string `json:"time"`
	Level string `json:"level"` // info / ok / warn / err
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Read  bool   `json:"read"`
	Count int    `json:"count,omitempty"` // 읽기 전에 같은 알림이 거듭 생긴 횟수 (2 이상일 때만)
}

// MaxNotices 는 남겨 두는 알림 수다 (넘으면 오래된 것부터 지운다).
const MaxNotices = 200

// NoticeLevels 는 알림 종류다.
var NoticeLevels = map[string]bool{"info": true, "ok": true, "warn": true, "err": true}

var noticesMu sync.Mutex

func noticesFile() string { return path("notices.json") }

// LoadNotices 는 알림 목록이다 (최근 것이 앞).
func LoadNotices() []Notice {
	noticesMu.Lock()
	defer noticesMu.Unlock()
	return loadNotices()
}

func loadNotices() []Notice {
	list := []Notice{} // 없으면 빈 목록 (API 가 null 대신 [] 를 돌려주게)
	_ = ReadJSON(noticesFile(), &list)
	if list == nil {
		list = []Notice{}
	}
	return list
}

// UnreadNotices 는 읽지 않은 알림 수다.
func UnreadNotices() int {
	n := 0
	for _, x := range LoadNotices() {
		if !x.Read {
			n++
		}
	}
	return n
}

// AddNotice 는 알림을 남긴다. 읽지 않은 같은 알림(종류·제목·내용)이 있으면 새로 쌓지 않고 시각과 횟수만 바꿔 맨 앞으로 올린다
// (한 시간마다 같은 폴더가 실패해도 알림이 쌓이지 않게).
func AddNotice(level, title, body string) {
	if !NoticeLevels[level] {
		level = "info"
	}
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if title == "" {
		return
	}
	noticesMu.Lock()
	defer noticesMu.Unlock()
	list := loadNotices()
	n := Notice{ID: NewID("n"), Time: Now(), Level: level, Title: title, Body: body, Count: 1}
	for i, x := range list {
		if !x.Read && x.Level == level && x.Title == title && x.Body == body {
			n.ID, n.Count = x.ID, max(x.Count, 1)+1
			list = append(list[:i], list[i+1:]...)
			break
		}
	}
	if n.Count < 2 {
		n.Count = 0
	}
	list = append([]Notice{n}, list...)
	if len(list) > MaxNotices {
		list = list[:MaxNotices]
	}
	_ = WriteJSON(noticesFile(), list)
}

// MarkNotices 는 알림을 읽음(read=true) 또는 안 읽음으로 바꾼다. all 이면 모두.
func MarkNotices(ids []string, all, read bool) error {
	return editNotices(func(list []Notice) []Notice {
		pick := idSet(ids)
		for i := range list {
			if all || pick[list[i].ID] {
				list[i].Read = read
			}
		}
		return list
	})
}

// DeleteNotices 는 알림을 지운다. all 이면 모두.
func DeleteNotices(ids []string, all bool) error {
	return editNotices(func(list []Notice) []Notice {
		if all {
			return []Notice{}
		}
		pick := idSet(ids)
		out := list[:0]
		for _, x := range list {
			if !pick[x.ID] {
				out = append(out, x)
			}
		}
		return out
	})
}

func editNotices(fn func([]Notice) []Notice) error {
	noticesMu.Lock()
	defer noticesMu.Unlock()
	return WriteJSON(noticesFile(), fn(loadNotices()))
}

func idSet(ids []string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}
