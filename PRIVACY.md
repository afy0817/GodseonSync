# GodseonSync 개인정보처리방침

시행일: 2026-10-19

GodseonSync 는 여러 PC 의 폴더를 사용자 본인의 구글 드라이브를 거쳐 동기화하는 데스크톱 프로그램입니다.
이 문서는 GodseonSync 가 구글 계정 데이터를 어떻게 다루는지 설명합니다.

GodseonSync 는 구글 드라이브 대신 사용자가 지정한 SFTP 서버를 저장소로 쓸 수도 있습니다. 이 경우 파일은 PC 와 그 서버 사이에서만
오가고, 서버 주소·사용자·(가려서 저장한) 비밀번호나 키 경로는 그 PC 의 `rclone.conf` 에만 저장됩니다. 아래 구글 관련 내용은 구글 드라이브 모드에 해당합니다.

## 접근하는 데이터

- **구글 드라이브 파일**: 사용자가 설정 화면에서 고른 폴더와, 내 드라이브의 `GodseonSync` 폴더
  (공용 설정 `setting.json`, PC 별 상태 `.status/`)의 파일을 읽고, 만들고, 고치고, 지웁니다.
  동기화에 필요한 일만 합니다.
- 구글 로그인 때 요청하는 권한은 구글 드라이브 범위(`drive` 또는 사용자가 고른 `drive.file`) 하나입니다.
  이름·이메일·연락처 같은 다른 구글 데이터는 요청하지 않습니다.

## 데이터가 오가는 곳과 저장되는 곳

- 파일은 **사용자의 PC 와 사용자의 구글 드라이브 사이에서만** 오갑니다. 개발자나 다른 서버로 보내지 않습니다.
- 구글 로그인 토큰은 그 PC 의 GodseonSync 데이터 폴더(`rclone.conf`)에만 저장됩니다.
- PC 이름·운영체제·앱 버전·마지막 동기화 시각 같은 상태 정보는 사용자 본인의 구글 드라이브
  `GodseonSync/.status/` 에 저장되어, 같은 계정을 쓰는 사용자의 다른 PC 만 읽습니다.
- 사용 통계, 광고, 분석 도구를 쓰지 않으며, 데이터를 팔거나 다른 사람과 공유하지 않습니다.

## 구글 사용자 데이터 사용 제한

GodseonSync 가 구글 API 에서 받은 정보의 사용과 다른 앱으로의 전송은
[Google API 서비스 사용자 데이터 정책](https://developers.google.com/terms/api-services-user-data-policy)의
제한적 사용(Limited Use) 요구 사항을 포함해 그 정책을 따릅니다.

## 접근 권한 거두기·데이터 지우기

- 설정 화면 › 설정 탭 › **구글 계정 연결 해제**를 누르면 그 PC 의 로그인 토큰이 지워집니다.
- 구글 계정의 [타사 앱 액세스 관리](https://myaccount.google.com/connections)에서 GodseonSync 의 권한을 언제든 거둘 수 있습니다.
- 앱과 데이터 폴더를 지우는 방법은 [README](README.md) 의 '데이터 위치' 에 있습니다.
  구글 드라이브의 파일은 사용자가 드라이브에서 직접 지웁니다.

## 바뀌는 경우

이 문서가 바뀌면 이 저장소의 `PRIVACY.md` 를 고치고 시행일을 바꿉니다.

## 문의

[GitHub 저장소의 Issues](https://github.com/afy0817/GodseonSync/issues) 로 문의하세요.

---

## Privacy Policy (English)

Effective: 2026-10-19

GodseonSync is a desktop program that syncs folders across your PCs through **your own** Google Drive.

- **Data accessed**: files in the folders you choose and in the `GodseonSync` folder of your My Drive
  (`setting.json`, `.status/`). It reads, creates, modifies and deletes them only as needed to sync.
  The only Google permission requested is the Google Drive scope (`drive`, or `drive.file` if you choose it).
- **Where data goes**: files move only between your PCs and your Google Drive. Nothing is sent to the developer or any other server.
  The sign-in token is stored only on that PC (`rclone.conf` in the GodseonSync data folder).
  PC status (name, OS, app version, last sync time) is stored in your own Drive (`GodseonSync/.status/`).
  No analytics, advertising or tracking; data is never sold or shared.
- **Limited Use**: GodseonSync's use and transfer of information received from Google APIs adheres to the
  [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy),
  including the Limited Use requirements.
- **Revoking access**: use Settings › Disconnect Google account in the app, or remove GodseonSync at
  [Google Account › Third-party connections](https://myaccount.google.com/connections).
- **Contact**: [GitHub Issues](https://github.com/afy0817/GodseonSync/issues).
