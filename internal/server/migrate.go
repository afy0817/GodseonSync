package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/afy0817/GodseonSync/internal/config"
	"github.com/afy0817/GodseonSync/internal/engine"
	"github.com/afy0817/GodseonSync/internal/i18n"
	"github.com/afy0817/GodseonSync/internal/rclone"
	"github.com/afy0817/GodseonSync/internal/storage"
	"github.com/afy0817/GodseonSync/internal/store"
)

// 구글 드라이브 권한 범위 옮기기 (드라이브 전체 → 이 앱이 만든 파일만).
//
// 새 범위(drive.file)로는 다른 클라이언트로 만든 지금 GodseonSync 폴더가 보이지 않는다. 그래서
//  1. 새 연결로 임시 이름 폴더(movingRoot)에 지금 설정을 새 세대(Epoch)로 만들고, 같은 계정인지 소유자를 비교한다.
//  2. 지금 연결로 GodseonSync 를 'GodseonSync (옮기기 전 …)' 으로 이름만 바꿔 남긴다 (지우지 않는다).
//  3. 새 연결로 임시 폴더를 GodseonSync 로 바꾸고, 연결을 바꿔 넣는다. 중간에 실패하면 되돌린다.
//
// 동기화한 파일은 옮기지 않는다. 세대가 바뀌어 모든 PC 가 폴더마다 한 번 합치면서(지우지 않음) 새 폴더에 다시 올린다.
// 옮기는 동안은 동기화 잠금을 잡고(예약 동기화가 건너뜀), 화면의 설정 받기·저장도 막는다 (s.migrating).

// movingRoot 는 옮기는 동안 새 연결에 만드는 임시 폴더 이름이다.
const movingRoot = config.RootFolder + "-옮기는-중"

// backupRoot 는 옮기기 전 폴더에 붙이는 이름이다.
func backupRoot(t time.Time) string {
	return i18n.T("%s (옮기기 전 %s)", config.RootFolder, t.Format("2006-01-02 15.04"))
}

// errMigrating 은 옮기는 동안 설정을 바꾸려 할 때의 오류다 (쓸 때마다 만들어 지금 언어로).
func errMigrating() error {
	return fail(http.StatusConflict, "저장소를 옮기는 중입니다. 끝난 뒤 다시 하세요.")
}

// lockForMigration 은 동기화 잠금을 잡는다. 동기화 중이면 끝날 때까지 기다린다 (최대 SwitchWait).
func lockForMigration(progress func(string)) (*store.Lock, error) {
	deadline := time.Now().Add(SwitchWait)
	for {
		lock, err := store.AcquireLock()
		if err == nil {
			lock.SetFolder(i18n.T("저장소 옮기는 중"))
			return lock, nil
		}
		if !errors.Is(err, store.ErrBusy) || time.Now().After(deadline) {
			return nil, fmt.Errorf(i18n.T("동기화 잠금을 잡지 못했습니다: %v"), err)
		}
		progress(i18n.T("지금 하는 동기화가 끝나기를 기다리는 중…"))
		time.Sleep(time.Second)
	}
}

// migrateScope 는 임시 이름(tmp)으로 로그인한 새 범위 연결로 저장소를 옮기고 원래 연결 자리에 넣는다.
func (s *Server) migrateScope(rc *rclone.Client, st store.Settings, tmp string, progress func(string)) error {
	s.migrating.Store(true)
	defer s.migrating.Store(false)
	lock, err := lockForMigration(progress)
	if err != nil {
		return err
	}
	defer lock.Release()

	newRoot := config.NewDrive(rc, tmp, config.RootFolder)
	progress(i18n.T("새 연결에서 GodseonSync 폴더를 찾는 중…"))
	if cfg, err := newRoot.Load(); err == nil {
		// 다른 PC 가 먼저 옮겼다: 그 설정에 이 PC 가 있으면 바꿔 넣기만 한다
		if cfg.FindPC(st.ClientID) == nil {
			return errors.New(i18n.T("새 연결의 setting.json 에 이 PC 가 없어 바꾸지 않았습니다 (지금 연결은 그대로입니다). 다른 계정의 GodseonSync 폴더일 수 있습니다."))
		}
		if err := rc.ReplaceRemote(tmp, st.Remote); err != nil {
			return err
		}
		store.Log("다른 PC 가 먼저 옮긴 GodseonSync 폴더로 연결을 바꿨습니다 (%s)", st.Remote)
		store.AddNotice("ok", i18n.T("다른 PC 가 먼저 옮긴 GodseonSync 폴더로 연결을 바꿨습니다"), "")
		return nil
	} else if !errors.Is(err, config.ErrNoSetting) {
		return fmt.Errorf(i18n.T("새 연결로 저장소를 읽지 못해 옮기지 않았습니다 (지금 연결은 그대로입니다): %v"), err)
	}

	progress(i18n.T("지금 설정을 읽는 중…"))
	oldRoot := engine.NewDrive(st)
	cfg, err := oldRoot.Load()
	if err != nil {
		return fmt.Errorf(i18n.T("지금 연결로 setting.json 을 읽지 못해 옮기지 않았습니다: %v"), err)
	}
	oldOwner, err := rc.Owner(oldRoot.Spec(oldRoot.SettingPath()))
	if err != nil {
		return fmt.Errorf(i18n.T("지금 setting.json 의 소유자를 읽지 못해 옮기지 않았습니다: %v"), err)
	}

	progress(i18n.T("새 연결에 설정을 만드는 중…"))
	moving := config.NewDrive(rc, tmp, movingRoot)
	if err := rc.Purge(moving.Spec(movingRoot)); err != nil { // 지난번에 끝내지 못하고 남은 것
		return fmt.Errorf(i18n.T("지난번 임시 폴더를 지우지 못했습니다: %v"), err)
	}
	removeMoving := func() { _ = rc.Purge(moving.Spec(movingRoot)) }
	cfg.Epoch = store.NewID("e")
	if _, err := moving.Create(cfg, st.ClientName); err != nil {
		removeMoving()
		return fmt.Errorf(i18n.T("새 연결에 setting.json 을 만들지 못해 옮기지 않았습니다: %v"), err)
	}
	newOwner, err := rc.Owner(moving.Spec(moving.SettingPath()))
	if err != nil {
		removeMoving()
		return fmt.Errorf(i18n.T("새 setting.json 의 소유자를 읽지 못해 옮기지 않았습니다: %v"), err)
	}
	if oldOwner != "" && newOwner != "" && oldOwner != newOwner {
		removeMoving()
		return errors.New(i18n.T("새로 로그인한 구글 계정이 지금 계정과 달라 옮기지 않았습니다 (지금 연결은 그대로입니다). 같은 계정으로 다시 로그인하세요."))
	}

	progress(i18n.T("GodseonSync 폴더를 바꾸는 중…"))
	backup := backupRoot(time.Now())
	if err := rc.MoveDir(oldRoot.Spec(config.RootFolder), oldRoot.Spec(backup)); err != nil {
		removeMoving()
		return fmt.Errorf(i18n.T("지금 GodseonSync 폴더 이름을 바꾸지 못해 옮기지 않았습니다: %v"), err)
	}
	if err := rc.MoveDir(moving.Spec(movingRoot), moving.Spec(config.RootFolder)); err != nil {
		if rerr := rc.MoveDir(oldRoot.Spec(backup), oldRoot.Spec(config.RootFolder)); rerr != nil {
			store.Log("저장소 옮기기 되돌리기 실패: 드라이브의 '%s' 를 %s 로 직접 바꾸세요 (%v)", backup, config.RootFolder, rerr)
		}
		removeMoving()
		return fmt.Errorf(i18n.T("새 GodseonSync 폴더를 만들지 못해 되돌렸습니다: %v"), err)
	}
	if err := rc.ReplaceRemote(tmp, st.Remote); err != nil {
		// 드라이브는 이미 옮겨졌다. 지금 연결(드라이브 전체)로도 새 폴더가 보이니 동기화는 이어진다.
		return fmt.Errorf(i18n.T("드라이브는 옮겼지만 연결을 바꾸지 못했습니다: %v"), err)
	}
	_ = store.WriteJSON(store.CacheFile(), cfg)
	store.AddNotice("ok", i18n.T("저장소를 '이 앱이 만든 파일만' 범위로 옮겼습니다"),
		i18n.T("예전 폴더는 드라이브의 '%s' 로 남겼습니다. 확인한 뒤 지워도 됩니다. 다음 동기화는 폴더마다 한 번 합칩니다.", backup))
	store.Log("저장소를 '이 앱이 만든 파일만' 범위로 옮겼습니다. 예전 폴더는 드라이브의 '%s' 로 남겼습니다 (확인한 뒤 지워도 됩니다). 다음 동기화는 폴더마다 한 번 합칩니다.", backup)
	return nil
}

// switchStorage 는 다른 저장소(모드)로 바꾼다. tmp 는 새 저장소 연결의 이름(새로 로그인한 임시 연결, 또는 저장해 둔 연결), m 은 새 모드다.
//
// 지금 저장소(드라이브·서버)의 내용은 건드리지 않는다 — 다시 돌아오면 마지막 상태 그대로 쓴다. 지금 연결은 savedName(지금 모드)로 저장해 둔다.
//   - 새 저장소에 이 PC 가 든 setting.json 이 있으면: 그 설정으로 연결만 바꾼다 (예전에 쓰던 저장소로 돌아오거나, 다른 PC 가 먼저 바꾼 경우).
//   - 새 저장소에 setting.json 이 없으면: 지금 설정을 새 세대로 복사해 만든 뒤 연결을 바꾼다.
//   - 이 PC 가 없는 다른 setting.json 이 있으면: 바꾸지 않는다 (다른 사람의 GodseonSync 일 수 있다).
//
// 세대가 다르므로 다음 동기화는 폴더마다 한 번 합친다 (양쪽에 파일이 있으면 '결정 필요').
func (s *Server) switchStorage(rc *rclone.Client, st store.Settings, tmp string, m *storage.Mode, progress func(string)) error {
	s.migrating.Store(true)
	defer s.migrating.Store(false)
	lock, err := lockForMigration(progress)
	if err != nil {
		return err
	}
	defer lock.Release()

	newRoot := config.NewDrive(rc, tmp, config.RootFolder)
	progress(i18n.T("새 저장소에서 GodseonSync 설정을 찾는 중…"))
	found, err := newRoot.Load()
	var cfg *config.Config
	switch {
	case err == nil && found.FindPC(st.ClientID) != nil:
		cfg = found
	case err == nil:
		return errors.New(i18n.T("새 저장소의 setting.json 에 이 PC 가 없어 바꾸지 않았습니다 (지금 연결은 그대로입니다). 다른 사람의 GodseonSync 폴더일 수 있습니다."))
	case !errors.Is(err, config.ErrNoSetting):
		return fmt.Errorf(i18n.T("새 저장소를 읽지 못해 바꾸지 않았습니다 (지금 연결은 그대로입니다): %v"), err)
	default:
		progress(i18n.T("지금 설정을 읽는 중…"))
		cur, err := engine.NewDrive(st).Load()
		if err != nil {
			return fmt.Errorf(i18n.T("지금 저장소의 setting.json 을 읽지 못해 바꾸지 않았습니다: %v"), err)
		}
		cur.Epoch = store.NewID("e")
		progress(i18n.T("새 저장소에 설정을 만드는 중…"))
		if cfg, err = newRoot.Create(cur, st.ClientName); err != nil {
			return fmt.Errorf(i18n.T("새 저장소에 setting.json 을 만들지 못해 바꾸지 않았습니다: %v"), err)
		}
	}

	// 지금 연결 → 저장, 새 연결 → 원래 이름. tmp 가 저장해 둔 같은 이름일 수 있어 잠깐 다른 이름으로 비켜 둔다.
	curKey := "unknown"
	if cm := storage.Of(rc, st.Remote); cm != nil {
		curKey = cm.Key
	} else if o, ok := rc.RemoteOptions(st.Remote); ok && o["type"] != "" {
		curKey = o["type"]
	}
	const swap = "gs-swap"
	if rc.HasRemote(swap) {
		_ = rc.DeleteRemote(swap)
	}
	if err := rc.ReplaceRemote(tmp, swap); err != nil {
		return err
	}
	if err := rc.ReplaceRemote(st.Remote, savedName(curKey)); err != nil {
		_ = rc.ReplaceRemote(swap, tmp)
		return fmt.Errorf(i18n.T("지금 연결을 저장하지 못해 바꾸지 않았습니다: %v"), err)
	}
	if err := rc.ReplaceRemote(swap, st.Remote); err != nil {
		_ = rc.ReplaceRemote(savedName(curKey), st.Remote)
		return fmt.Errorf(i18n.T("새 연결로 바꾸지 못해 되돌렸습니다: %v"), err)
	}
	_ = store.WriteJSON(store.CacheFile(), cfg)
	where := ""
	if conf, ok := rc.RemoteOptions(st.Remote); ok {
		where = m.Where(conf)
	}
	store.Log("저장소를 %s(%s)로 바꿨습니다. 지금까지 쓰던 연결은 저장해 두었고, 그 저장소는 그대로입니다.", m.Label, where)
	store.AddNotice("ok", i18n.T("저장소를 %s(%s)로 바꿨습니다", i18n.T(m.Label), where),
		i18n.T("지금까지 쓰던 저장소와 연결은 그대로 두었습니다 (설정 › 모드에서 로그인 없이 돌아갈 수 있습니다). 다른 PC 는 각자 모드를 바꿔야 같은 저장소를 씁니다. 다음 동기화는 폴더마다 한 번 합칩니다."))
	return nil
}
