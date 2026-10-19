# GodseonSync

여러 PC 의 폴더를 **구글 드라이브나 SFTP 서버(NAS 등)를 거쳐 폴더 구조 그대로** 양방향 동기화하는 프로그램입니다.

- **추가 설치 없음** — 내려받은 프로그램 하나만 있으면 됩니다.
- **PC 마다 다른 경로** — 같은 폴더를 맥에서는 `~/문서/프로젝트`, 회사 윈도우에서는 `D:\프로젝트`, 집 윈도우에서는 `E:\work\프로젝트` 처럼
  PC 마다 다른 곳에 둘 수 있습니다. 같은 윈도우라도 PC 가 다르면 각자 따로 등록됩니다.
- **설정은 한 번만** — 폴더 목록과 PC 별 경로는 저장소에 저장되어, 한 PC 에서 바꾸면 다른 PC 도 다음 동기화 때 그대로 받습니다.
- **백그라운드 실행** — 메뉴 막대(맥)·알림 영역(윈도우)에 아이콘으로 떠 있으면서 정해진 주기마다 동기화합니다.
- 중간 저장소를 거치므로 PC 끼리 동시에 켜져 있을 필요가 없습니다.
- 한국어 / English

```
 맥북                  구글 드라이브 (또는 SFTP 서버)            회사 윈도우          집 윈도우
 ~/문서/프로젝트  ⇄    GodseonSync/프로젝트              ⇄     D:\프로젝트          E:\work\프로젝트
                       GodseonSync/setting.json  ← 모든 PC 가 함께 쓰는 설정
```

## 설치

[Releases](https://github.com/afy0817/GodseonSync/releases) 에서 내려받습니다.

| OS | 파일 | 설치 |
|---|---|---|
| macOS 13 이상 (Intel·Apple Silicon) | `GodseonSync-<버전>-mac.dmg` | 열어서 GodseonSync 를 **응용 프로그램** 폴더로 끌어 놓습니다 |
| Windows 10/11 (64비트) | `GodseonSync-<버전>-windows-setup.exe` | 실행합니다 (관리자 권한 필요 없음) |
| Windows (설치 없이) | `GodseonSync-<버전>-windows-portable.zip` | 원하는 폴더에 풀고 `GodseonSync.exe` 를 실행합니다 |
| Linux (amd64·arm64) | `GodseonSync-<버전>-linux-<아키텍처>.tar.gz` | 풀고 `./godseonsync` 를 실행합니다 |

### 처음 실행할 때 경고가 뜨면

개인 프로젝트라 애플·마이크로소프트 개발자 인증서로 서명하지 않았습니다. 처음 한 번만 아래처럼 허용하면 됩니다.

- **맥**: "확인되지 않은 개발자" 창이 뜨면 닫고, **시스템 설정 › 개인정보 보호 및 보안** 아래쪽의
  "GodseonSync 이(가) 차단되었습니다" 옆 **그래도 열기**를 누릅니다.
  - 동기화할 폴더가 문서·데스크탑 등에 있으면 "GodseonSync 가 ○○ 폴더에 접근하려고 함" 창에서 **허용**을 누릅니다.
  - "GodseonSync 에서 알림을 보내려고 합니다" 가 뜨면 **허용**을 누릅니다 (동기화 실패·결정 필요를 알려 줍니다).
- **윈도우**: "Windows의 PC 보호" 창이 뜨면 **추가 정보 › 실행**을 누릅니다.
  설정 창은 Microsoft Edge WebView2 를 씁니다 (Windows 10/11 에 기본으로 들어 있음). 없으면 기본 브라우저로 열립니다.

## 처음 설정

GodseonSync 를 실행하면 설정 창이 열립니다.

1. **저장소 연결** — **구글 드라이브** 또는 **SFTP** 를 고릅니다. 로그인 정보는 그 PC 에만 저장됩니다.
   - 구글 드라이브: **구글 계정으로 연결**을 누르고, 브라우저에서 로그인한 뒤 **허용**을 누릅니다.
   - SFTP: 서버 정보를 넣고 **연결**을 누릅니다 (아래 [SFTP 모드](#sftp-모드)). 실제로 접속되는지 확인한 뒤에만 연결됩니다.
2. **이 PC 이름** — 저장소에 `GodseonSync` 폴더가 자동으로 만들어집니다 (이미 있으면 그대로 씁니다). 이 PC 의 이름을 정합니다.
   - 다른 PC 에서 이미 쓰고 있으면 **새 PC 로 등록**하거나, 다시 설치한 PC 라면 예전 PC 를 **이어받기** 합니다.
   - **모든 PC 가 같은 구글 계정(또는 같은 SFTP 서버·사용자·폴더)으로 연결해야** 같은 설정을 함께 씁니다.
3. **백그라운드 실행** — 컴퓨터에 로그인할 때 자동으로 실행할지 고릅니다.

## 동기화할 폴더 추가

왼쪽 메뉴 **폴더 › 폴더 추가**에서 만듭니다. (화면의 '드라이브' 는 SFTP 모드에서 'SFTP' 로 나옵니다.)

- **드라이브 경로**: 저장소 안의 위치입니다 (기본 `GodseonSync/<폴더 이름>`). **찾아보기**로 이미 있는 폴더를 고를 수도 있습니다.
- **이 PC 경로**: 이 PC 에서 동기화할 폴더입니다. 비워 두면 이 PC 에서는 그 폴더를 동기화하지 않습니다.
- **다른 PC 경로**: 다른 PC 의 경로도 여기서 넣을 수 있습니다. 다른 PC 는 거기에 GodseonSync 를 설치하고 같은 계정으로 연결하면 목록에 나타납니다.
- **제외 규칙**: 동기화하지 않을 파일을 한 줄에 하나씩 적습니다. `node_modules/` (폴더), `*.tmp` (패턴).
  모든 폴더에 쓰는 공통 규칙은 **설정 › 제외 규칙**에 있습니다.

**드라이브에 저장**을 누르면 설정이 저장소에 올라가고 바로 동기화합니다.
두 PC 에서 동시에 설정을 고치면 나중에 저장하는 쪽에 "다른 PC 에서 설정이 바뀌었습니다" 가 뜨고, 최신 설정을 다시 불러온 뒤 저장하게 됩니다.

> 설정은 설정 창에서 고치세요. 드라이브 웹에서 `setting.json` 을 직접 고치다 형식이 깨지면 동기화가 멈춥니다.
> 저장소의 `GodseonSync` 폴더(`setting.json`, `.status`)는 지우지 마세요.

## 동기화 규칙

- 추가·수정·삭제·폴더 구조·빈 폴더가 모두 양쪽에 반영됩니다.
- **처음 동기화(합치기)는 아무것도 지우지 않습니다.** 양쪽에 같은 파일이 있으면 더 최신 파일을 남깁니다.
  폴더 경로나 제외 규칙을 바꿨을 때도 합치기부터 합니다.
- 합치기 전에 양쪽을 비교해서, **양쪽 파일이 같으면 묻지 않고 합칩니다.**
- **양쪽이 다르면 그 폴더만 멈추고 '결정 필요'** 로 알려 줍니다. 폴더 카드에 '이 PC 에만 N개, 드라이브에만 N개, 내용이 다른 파일 N개' 와
  예시 파일 이름(**다른 파일 보기**)이 나옵니다. 셋 중 하나를 고릅니다.
  - **드라이브 → 이 PC 덮어쓰기** / **이 PC → 드라이브 덮어쓰기**: 덮어쓰기 전에 덮어쓸 쪽을 옆에 `<폴더 이름>_backup_년월일시분초` 로
    통째로 복사해 둡니다 (예: `문서_backup_20261019134034`). 확인한 뒤 필요 없으면 지우세요.
  - **합치기**: 양쪽 파일을 모두 남깁니다.
  - 한쪽이 비어 있으면 묻지 않고 합칩니다.
- **충돌**: 두 PC 에서 같은 파일을 고쳤으면 최신 파일이 이기고, 다른 쪽은 `이름.conflict1` 처럼 이름을 바꿔 남깁니다.
- **안전장치**: 한 번에 절반 넘게 지워지려 하면 멈추고 알려 줍니다. 일부러 지운 거라면 폴더 카드의 **강제 실행**,
  실수였다면 **합치기**로 지운 파일을 되살립니다.
- 구글 문서·스프레드시트 같은 구글 전용 파일은 동기화하지 않습니다.
- 맥의 한글 파일 이름도 윈도우에서 깨지지 않게 올립니다.

## 화면 안내

설정 창 왼쪽 메뉴에서 화면을 고릅니다. 메뉴는 접을 수 있습니다 (접으면 아이콘만, 펼치면 아이콘과 이름).

| 메뉴 | 내용 |
|---|---|
| **폴더** | 동기화 폴더 목록과 폴더마다 드라이브 경로·PC 별 경로·제외 규칙, 상태 |
| **등록 PC** | 같은 저장소를 쓰는 PC 목록과 각 PC 의 마지막 동기화 (이름 바꾸기, 목록에서 빼기) |
| **설정** | **일반**(언어, 자동 실행, 일시 정지, 동기화 주기) · **제외 규칙** · **모드**(구글 드라이브/SFTP, 다시 연결, 저장소 바꾸기) · **이 PC**(처음 설정 다시 하기, 연결 해제, 종료) · **로그** |

- **알림 센터**: 상단 오른쪽 종 아이콘. 동기화 실패, 결정이 필요한 폴더, 덮어쓰기(백업 위치) 같은 일이 시간 순서로 남고,
  읽지 않은 수가 종에 표시됩니다. 읽음·안 읽음·지우기를 할 수 있습니다. 설정 창을 닫아 둔 사이에 생긴 일도 남습니다.
- **OS 알림**: 동기화 실패·결정 필요는 화면 구석 알림으로도 알려 주고, 알림을 누르면 설정 창이 열립니다.
  맥에서 알림이 안 뜨면 **시스템 설정 › 알림 › GodseonSync** 에서 **알림 허용**을 켭니다.
- **언어**: **설정 › 일반 › 언어**에서 **한국어 / English** 를 고릅니다. 처음에는 시스템 언어를 따릅니다.
- 설정 창은 마지막으로 받아 둔 설정으로 바로 열리고, 최신 설정은 뒤에서 받습니다. 화면 제목 오른쪽에 '설정: 3분 전 받음' 처럼 보이고,
  **설정 동기화**를 누르면 바로 다시 받습니다. 저장소에 연결할 수 없을 때도 마지막으로 받은 설정을 보여 줍니다.

### 트레이 메뉴

| 메뉴 | 하는 일 |
|---|---|
| 설정 열기… | 설정 창을 엽니다 (윈도우는 아이콘을 눌러도 열림) |
| 지금 동기화 | 모든 폴더를 바로 동기화합니다 |
| 자동 동기화 일시 정지 | 주기 동기화를 멈춥니다 (지금 동기화는 됨) |
| GodseonSync 종료 | 백그라운드 동기화까지 끝냅니다 |

설정 창을 닫아도 백그라운드 동기화는 계속됩니다.

## 저장소

### 구글 드라이브

구글 계정으로 로그인만 하면 됩니다. GodseonSync 는 **'이 앱이 만든 파일만'** 권한으로 연결합니다.
그래서 내 드라이브의 다른 파일은 보지 않고, GodseonSync 가 만든 폴더와 파일만 다룹니다.

- 드라이브 웹에서 `GodseonSync` 폴더에 **직접 끌어다 넣은 파일은 GodseonSync 에 보이지 않습니다.** 파일은 PC 의 동기화 폴더에 넣으세요.
- 모든 PC 가 같은 구글 계정으로 로그인해야 합니다.
- 고급 설정에서 **'드라이브 전체'** 권한을 고를 수도 있지만, 로그인할 때 '확인되지 않은 앱' 경고가 뜹니다.
- 예전 버전에서 '드라이브 전체' 로 쓰던 PC 는 **설정 › 모드 › 이 앱이 만든 파일만으로 옮기기**를 누르고 같은 계정으로 로그인합니다.
  지금 `GodseonSync` 폴더는 `GodseonSync (옮기기 전 날짜 시각)` 으로 이름만 바꿔 남기고, 새 폴더로 다시 올립니다.
  옛 폴더는 확인한 뒤 드라이브 웹에서 지우세요.

### 구글 OAuth 클라이언트

배포판에는 GodseonSync 의 구글 클라이언트가 들어 있어서 따로 할 일이 없습니다.
직접 만든 구글 클라이언트를 쓰고 싶을 때만 아래를 따릅니다 (무료, 한 번만).

1. [Google Cloud Console](https://console.cloud.google.com) 에서 프로젝트를 만들고 **API 및 서비스 › 라이브러리 › Google Drive API › 사용**을 누릅니다.
   빼먹으면 로그인 뒤 '… Drive API has not been used in project …' 로 실패합니다.
2. **Google 인증 플랫폼(OAuth 동의 화면)**: 사용자 유형 '외부', 앱 이름·지원 이메일·개발자 연락처를 넣습니다.
   **브랜딩**에 홈페이지·개인정보처리방침 링크와 그 도메인을 넣고 저장한 뒤, **대상 › 앱 게시 › 프로덕션으로 게시**를 누릅니다.
   테스트 상태로 두면 테스트 사용자로 넣은 계정만 로그인할 수 있고('403 access_denied'), 7일마다 다시 로그인해야 합니다.
3. **사용자 인증 정보 › OAuth 클라이언트 ID 만들기 › 데스크톱 앱** 으로 클라이언트 ID 와 보안 비밀을 받습니다.
4. PC 마다 설정 창에 넣습니다.
   - 처음 설정: **저장소 연결**의 고급 설정에 넣고 **구글 계정으로 연결**.
   - 이미 쓰는 PC: **설정 › 모드**에 넣고 **입력한 정보로 다시 연결**. 로그인을 취소하거나 실패하면 지금 연결을 그대로 씁니다.

**모든 PC 가 같은 클라이언트를 써야 합니다.** '이 앱이 만든 파일만' 권한은 같은 클라이언트로 만든 파일만 보이기 때문입니다.

### SFTP 모드

NAS·리눅스 서버처럼 SSH(SFTP)로 접속할 수 있는 곳을 저장소로 씁니다. 서버에 따로 설치할 것은 없습니다.

| 칸 | 내용 |
|---|---|
| 호스트 · 포트 | 서버 이름이나 IP (예: `nas.local`, `192.168.0.10`). 포트는 기본 22 |
| 사용자 | SSH 사용자 |
| 로그인 방식 | **비밀번호**, 또는 **키 파일**(예: `~/.ssh/id_ed25519`, 키에 암호가 있으면 키 암호도) |
| 폴더 위치 | 비우면 그 사용자의 홈 폴더. 그 아래에 `GodseonSync` 폴더를 만듭니다. 맨 위 폴더에 쓸 수 없는 서버(많은 NAS)는 쓸 수 있는 하위 폴더(예: `upload`)나 전체 경로(예: `/volume1/sync`)를 넣습니다 |

- 비밀번호·키 암호는 그 PC 에만 가려서 저장되고, 화면에 다시 보이지 않습니다.
- 모든 PC 가 **같은 서버·같은 사용자·같은 폴더 위치**로 연결해야 같은 설정을 함께 씁니다.
- 서버의 호스트 키는 확인하지 않습니다. 믿을 수 있는 네트워크의 서버를 쓰세요.

### 저장소 바꾸기 (예: 구글 드라이브 → SFTP)

**설정 › 모드**에서 다른 모드를 고르고 값을 넣은 뒤 **이 저장소로 바꾸기**를 누릅니다. **이 PC 만** 바뀝니다.

- 지금까지 쓰던 저장소의 파일과 설정은 **건드리지 않고 그대로** 둡니다.
- 지금까지 쓰던 연결은 **저장된 연결**로 남습니다. **저장된 연결로 바꾸기**를 누르면 다시 로그인하지 않고 돌아갑니다.
- 새 저장소에 GodseonSync 설정이 이미 있으면 그 설정을 쓰고, 없으면 지금 설정을 복사해 만듭니다.
- 파일은 다음 동기화 때 폴더마다 한 번 합치며 새 저장소로 올라갑니다 (양쪽에 다른 파일이 있으면 '결정 필요').
- 다른 PC 도 같은 저장소를 쓰려면 각자 모드를 바꿉니다.

## 명령줄

트레이 없이 쓰거나(리눅스 서버 등) 문제를 확인할 때 씁니다.

```
GodseonSync                    실행 (트레이 아이콘 + 설정 창)
GodseonSync sync [폴더 ...]     지금 동기화  (--dry-run 미리보기, --resync 합치기, --force 대량 삭제 허용)
GodseonSync status             설정과 마지막 결과 보기
GodseonSync log [-n 100]       최근 로그 보기 (--clear 로그 지우기)
GodseonSync autostart on|off   로그인할 때 자동 실행 켜기/끄기
GodseonSync quit               실행 중인 GodseonSync 끝내기
```

맥에서는 `/Applications/GodseonSync.app/Contents/MacOS/GodseonSync`, 윈도우에서는 설치 폴더의 `GodseonSync.exe` 입니다.

## 데이터 위치

PC 마다 아래 폴더에 이 PC 의 설정, 동기화 기록, 로그, 로그인 정보가 있습니다. 저장소에는 올라가지 않습니다.

- 맥: `~/Library/Application Support/GodseonSync`
- 윈도우: `%LOCALAPPDATA%\GodseonSync`
- 리눅스: `~/.local/state/godseonsync`

**완전히 지우기**

1. 트레이 메뉴 › **GodseonSync 종료**
2. 로그인 자동 실행 끄기: 설정 › 일반, 또는 `GodseonSync autostart off` (윈도우 설치판은 제거 프로그램이 해 줍니다)
3. 앱을 지우고, 위의 데이터 폴더를 지웁니다.
4. 모든 PC 에서 그만 쓸 때만 저장소의 `GodseonSync/setting.json`, `GodseonSync/.status` 를 지웁니다.

동기화한 파일은 PC·저장소 어디에서도 지워지지 않습니다.

## 개인정보

GodseonSync 는 파일을 사용자의 PC 와 사용자 본인의 저장소(구글 드라이브·SFTP 서버) 사이에서만 옮기고, 개발자나 다른 서버로 보내지 않습니다.
자세한 내용은 [개인정보처리방침](PRIVACY.md) 에 있습니다.

## 라이선스

[MIT](LICENSE). 프로그램에 들어 있는 [rclone](https://github.com/rclone/rclone) (MIT) 등 오픈 소스 구성 요소의 라이선스는
배포 파일의 `THIRD_PARTY_LICENSES.txt` 에 있습니다.

---

## English

GodseonSync keeps folders on multiple PCs in two-way sync through Google Drive or an SFTP server (NAS etc.), preserving the folder structure.

- **Nothing else to install** — download one program.
- **Different paths per PC** — the same folder can live at `~/Documents/project` on a Mac and `D:\project` on a Windows PC.
  Each PC is registered on its own, even two Windows PCs.
- **Set up once** — the folder list and per-PC paths are stored in the storage (`GodseonSync/setting.json`), so every PC picks up changes.
- **Runs in the background** from the menu bar / system tray. PCs don't need to be on at the same time.
- **Safe first sync** — the first sync never deletes anything. If both sides already have different files, that folder pauses and asks
  which way to go (drive → this PC, this PC → drive, or merge); the side being overwritten is backed up as `<folder>_backup_<timestamp>` first.
- Notification center (bell icon), system notifications, Korean and English UI (Settings › General › Language).

Download from [Releases](https://github.com/afy0817/GodseonSync/releases). On first launch, macOS may say the developer can't be verified:
open **System Settings › Privacy & Security** and click **Open Anyway**. On Windows, click **More info › Run anyway**.
Files move only between your PCs and your own storage — see the [privacy policy](PRIVACY.md).
