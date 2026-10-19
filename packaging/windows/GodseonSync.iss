; GodseonSync 윈도우 설치 파일 (Inno Setup 6)
; 먼저 scripts/build.sh windows 로 build\windows-amd64 를 만든 뒤:
;   iscc /DAppVersion=1.2.3 packaging\windows\GodseonSync.iss
; 관리자 권한 없이 사용자 폴더(%LOCALAPPDATA%\Programs\GodseonSync)에 설치한다.

#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
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
