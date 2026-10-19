; GodseonSync 윈도우 설치 파일 (Inno Setup 6)
; 먼저 scripts/build.sh windows 로 build\windows-amd64 를 만든 뒤:
;   iscc /DAppVersion=1.2.3 packaging\windows\GodseonSync.iss          (미리 릴리스는 /DAppVersion=1.2.3-beta /DAppNumVersion=1.2.3)
; 관리자 권한 없이 사용자 폴더(%LOCALAPPDATA%\Programs\GodseonSync)에 설치한다.
;
; /DSignUninstaller: 제거 프로그램을 밖에서 서명한다 (GitHub Actions 의 SignPath 서명). 서명 도구 없이 SignedUninstaller=yes 면
; 처음 컴파일은 build\uninst\uninst-<Inno 버전>-<해시>.e32 를 만들고 '서명 필요' 로 멈춘다 → 그 파일을 서명해 두고 다시 컴파일하면
; Inno Setup 이 서명을 확인해(서명만 빼면 원래와 같은지) 끼워 넣는다. 버전 정보가 바뀌면 해시도 바뀌어 릴리스마다 새로 서명한다.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
; 윈도우 버전 정보는 숫자만 받는다: 태그가 0.2.0-beta 면 /DAppNumVersion=0.2.0 (Actions 가 넘긴다 — exe 의 버전 정보·SignPath 에 넘기는 버전과 같게)
#ifndef AppNumVersion
  #define AppNumVersion AppVersion
#endif

[Setup]
AppId={{8C1B5E2A-6F7D-4C3B-9A1E-2D4F6B8A0C11}
AppName=GodseonSync
AppVersion={#AppVersion}
AppVerName=GodseonSync {#AppVersion}
AppPublisher=afy0817
AppPublisherURL=https://github.com/afy0817/GodseonSync
AppSupportURL=https://github.com/afy0817/GodseonSync/issues
DefaultDirName={localappdata}\Programs\GodseonSync
DisableProgramGroupPage=yes
DisableDirPage=auto
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..\..\dist
OutputBaseFilename=GodseonSync-{#AppVersion}-windows-setup
SetupIconFile=..\..\build\icons\app.ico
UninstallDisplayIcon={app}\GodseonSync.exe
UninstallDisplayName=GodseonSync
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
; 설치 파일·제거 프로그램의 버전 정보 (코드 서명이 제품 이름·버전을 확인한다)
VersionInfoVersion={#AppNumVersion}
VersionInfoTextVersion={#AppNumVersion}
VersionInfoProductName=GodseonSync
VersionInfoProductVersion={#AppNumVersion}
VersionInfoProductTextVersion={#AppNumVersion}
VersionInfoCompany=afy0817
VersionInfoDescription=GodseonSync Setup
VersionInfoCopyright=Copyright (c) 2026 afy0817. MIT License.
#ifdef SignUninstaller
SignedUninstaller=yes
SignedUninstallerDir=..\..\build\uninst
#endif

[Languages]
#ifexist AddBackslash(CompilerPath) + "Languages\Korean.isl"
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"
#endif
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "..\..\build\windows-amd64\GodseonSync.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\build\windows-amd64\LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\build\windows-amd64\THIRD_PARTY_LICENSES.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{userprograms}\GodseonSync"; Filename: "{app}\GodseonSync.exe"

[Registry]
; 앱이 실행될 때 이 사용자 레지스트리에 등록하는 알림 앱 이름·아이콘(AppUserModelId)과, 알림을 누르면 설정 창을 여는 godseonsync: 주소.
; 설치할 때는 만들지 않고(dontcreatekey) 제거할 때 지운다 (uninsdeletekey)
Root: HKCU; Subkey: "Software\Classes\AppUserModelId\GodseonSync"; Flags: uninsdeletekey dontcreatekey
Root: HKCU; Subkey: "Software\Classes\godseonsync"; Flags: uninsdeletekey dontcreatekey

[Run]
; 설명 문구는 Inno Setup 기본 번역(LaunchProgram)을 쓴다 — 이 파일은 BOM 없는 UTF-8 이라 한글을 직접 쓰면 깨질 수 있다
Filename: "{app}\GodseonSync.exe"; Description: "{cm:LaunchProgram,GodseonSync}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; 실행 중인 GodseonSync 를 끝내고, 로그인 자동 실행 등록을 지운다 (동기화한 파일과 설정은 그대로 둔다)
Filename: "{app}\GodseonSync.exe"; Parameters: "quit"; Flags: runhidden waituntilterminated; RunOnceId: "GodseonSyncQuit"
Filename: "{app}\GodseonSync.exe"; Parameters: "autostart off"; Flags: runhidden waituntilterminated; RunOnceId: "GodseonSyncAutostartOff"

[Code]
// 업데이트 설치 전에 실행 중인 GodseonSync 를 끝내야 실행 파일을 바꿀 수 있다
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  rc: Integer;
  exe: String;
begin
  exe := ExpandConstant('{app}\GodseonSync.exe');
  if FileExists(exe) then
  begin
    Exec(exe, 'quit', '', SW_HIDE, ewWaitUntilTerminated, rc);
    Sleep(1500);
  end;
  Result := '';
end;
