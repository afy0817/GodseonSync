package store

import "testing"

func TestNotices(t *testing.T) {
	t.Setenv("GODSEONSYNC_HOME", t.TempDir())
	AddNotice("err", "[문서] 동기화 실패", "없음")
	AddNotice("warn", "[사진] 골라 주세요", "")
	AddNotice("err", "[문서] 동기화 실패", "없음") // 읽기 전에 같은 알림 → 합친다
	AddNotice("bogus", "  ", "")          // 제목이 없으면 남기지 않는다
	list := LoadNotices()
	if len(list) != 2 || list[0].Title != "[문서] 동기화 실패" || list[0].Count != 2 || list[1].Count != 0 || UnreadNotices() != 2 {
		t.Fatalf("같은 알림은 합쳐 맨 앞으로: %+v", list)
	}
	if err := MarkNotices([]string{list[0].ID}, false, true); err != nil || UnreadNotices() != 1 {
		t.Fatal("읽음 표시")
	}
	AddNotice("err", "[문서] 동기화 실패", "없음") // 읽은 뒤에는 새 알림
	if len(LoadNotices()) != 3 {
		t.Fatal("읽은 알림과는 합치지 않는다")
	}
	_ = MarkNotices(nil, true, false)
	if UnreadNotices() != 3 {
		t.Fatal("모두 안 읽음으로")
	}
	_ = DeleteNotices([]string{LoadNotices()[0].ID}, false)
	if len(LoadNotices()) != 2 {
		t.Fatal("하나 지우기")
	}
	for i := 0; i < MaxNotices+20; i++ {
		AddNotice("info", "알림", string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	if len(LoadNotices()) != MaxNotices {
		t.Fatalf("최대 %d개: %d", MaxNotices, len(LoadNotices()))
	}
	_ = DeleteNotices(nil, true)
	if len(LoadNotices()) != 0 {
		t.Fatal("모두 지우기")
	}
}
