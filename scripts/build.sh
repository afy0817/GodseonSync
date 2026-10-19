#!/usr/bin/env bash
# GodseonSync 배포 파일을 만든다.
#
#   scripts/build.sh mac       맥 유니버설 앱 → dist/GodseonSync-<버전>-mac.dmg   (맥에서만)
#   scripts/build.sh windows   윈도우 실행 파일 → build/windows-amd64/GodseonSync.exe, dist/...-windows-portable.zip
#                              설치 파일은 윈도우에서 Inno Setup 으로 packaging/windows/GodseonSync.iss 를 컴파일한다
#   scripts/build.sh windows-zip  build/windows-amd64 로 portable zip 만 다시 만든다 (Actions 에서 exe 를 서명한 뒤)
#   scripts/build.sh linux     리눅스 → dist/GodseonSync-<버전>-linux-<아키텍처>.tar.gz
#
# 환경 변수
#   VERSION                     버전 (기본: git 태그, 없으면 0.0.0-dev)
#   GS_GOOGLE_CLIENT_ID         구글 OAuth 클라이언트 ID (선택, README 참고)
#   GS_GOOGLE_CLIENT_SECRET     구글 OAuth 클라이언트 보안 비밀 (선택)
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo 0.0.0-dev)}"
VERSION="${VERSION#v}"
PKG=github.com/afy0817/GodseonSync/internal/buildinfo
LDFLAGS="-s -w -X ${PKG}.Version=${VERSION}"
if [[ -n "${GS_GOOGLE_CLIENT_ID:-}" ]]; then
  # 값은 출력하지 않는다. 다른 것(예: 복사 명령 글자)이 들어가면 링커가 엉뚱한 옵션으로 읽으니 형식부터 본다.
  if [[ ! "$GS_GOOGLE_CLIENT_ID" =~ ^[0-9]+-[0-9a-z]+\.apps\.googleusercontent\.com$ ]]; then
    echo "GS_GOOGLE_CLIENT_ID 가 구글 클라이언트 ID 형식(숫자-…apps.googleusercontent.com)이 아닙니다 (길이 ${#GS_GOOGLE_CLIENT_ID})" >&2
    exit 1
  fi
  if [[ -z "${GS_GOOGLE_CLIENT_SECRET:-}" || "$GS_GOOGLE_CLIENT_SECRET" =~ [[:space:]\'\"] ]]; then
    echo "GS_GOOGLE_CLIENT_SECRET 이 비었거나 공백·따옴표가 들어 있습니다 (길이 ${#GS_GOOGLE_CLIENT_SECRET})" >&2
    exit 1
  fi
  LDFLAGS+=" -X ${PKG}.GoogleClientID=${GS_GOOGLE_CLIENT_ID} -X ${PKG}.GoogleClientSecret=${GS_GOOGLE_CLIENT_SECRET}"
fi

mkdir -p build dist
echo "GodseonSync ${VERSION}"

icons() {
  go run ./tools/genicons -out build/icons
}

licenses() { # <실행 파일> <출력>
  go run ./tools/licenses "$1" "$2"
}

build_mac() {
  icons
  export MACOSX_DEPLOYMENT_TARGET=13.0
  for arch in arm64 amd64; do
    echo "== macOS ${arch}"
    CGO_ENABLED=1 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "$LDFLAGS" -o "build/mac-${arch}/GodseonSync" .
  done
  local app=build/mac/GodseonSync.app
  rm -rf build/mac && mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  lipo -create -output "$app/Contents/MacOS/GodseonSync" build/mac-arm64/GodseonSync build/mac-amd64/GodseonSync
  sed "s/__VERSION__/${VERSION}/g" packaging/macos/Info.plist > "$app/Contents/Info.plist"
  iconutil -c icns build/icons/AppIcon.iconset -o "$app/Contents/Resources/AppIcon.icns"
  licenses build/mac-arm64/GodseonSync "$app/Contents/Resources/THIRD_PARTY_LICENSES.txt"
  cp LICENSE "$app/Contents/Resources/LICENSE.txt"
  # 개발자 인증서가 없어 임시(ad-hoc) 서명만 한다. 처음 열 때 README 의 '처음 실행' 안내를 따른다.
  codesign --force --deep --sign - "$app"

  local stage=build/dmg
  rm -rf "$stage" && mkdir -p "$stage"
  cp -R "$app" "$stage/"
  ln -s /Applications "$stage/Applications"
  local dmg="dist/GodseonSync-${VERSION}-mac.dmg"
  rm -f "$dmg"
  hdiutil create -volname "GodseonSync" -srcfolder "$stage" -ov -format UDZO "$dmg" >/dev/null
  echo "→ $dmg"
}

build_windows() {
  icons
  local arch=amd64 out=build/windows-amd64
  mkdir -p "$out"
  # 매니페스트(고해상도 화면, 긴 경로, 1번)·앱 아이콘(2번 — 설정 창이 이 번호로 찾는다)·버전 정보(제품 이름·버전 —
  # 코드 서명(SignPath)이 이 값을 확인한다)를 실행 파일에 넣는다. 윈도우 버전 숫자는 '0.1.0-beta' 의 '0.1.0' 부분만.
  local winver="${VERSION%%[!0-9.]*}"
  go run github.com/tc-hib/go-winres@v0.3.3 make --in packaging/windows/winres.json --arch $arch \
    --product-version "${winver:-0.0.0}" --file-version "${winver:-0.0.0}" --out rsrc
  echo "== Windows ${arch}"
  CGO_ENABLED=0 GOOS=windows GOARCH=$arch go build -trimpath -ldflags "$LDFLAGS -H windowsgui" -o "$out/GodseonSync.exe" .
  rm -f rsrc_windows_${arch}.syso
  licenses "$out/GodseonSync.exe" "$out/THIRD_PARTY_LICENSES.txt"
  cp LICENSE "$out/LICENSE.txt"
  zip_windows
}

zip_windows() {
  local out=build/windows-amd64
  local zip="dist/GodseonSync-${VERSION}-windows-portable.zip"
  rm -f "$zip"
  if command -v zip >/dev/null; then
    (cd "$out" && zip -q -r "../../$zip" GodseonSync.exe LICENSE.txt THIRD_PARTY_LICENSES.txt)
  else
    powershell -NoProfile -Command "Compress-Archive -Force -Path '$out/GodseonSync.exe','$out/LICENSE.txt','$out/THIRD_PARTY_LICENSES.txt' -DestinationPath '$zip'"
  fi
  echo "→ $zip"
}

build_linux() {
  for arch in amd64 arm64; do
    echo "== Linux ${arch}"
    local out="build/linux-${arch}/GodseonSync"
    mkdir -p "$out"
    CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags "$LDFLAGS" -o "$out/godseonsync" .
    licenses "$out/godseonsync" "$out/THIRD_PARTY_LICENSES.txt"
    cp LICENSE README.md "$out/"
    tar -C "build/linux-${arch}" -czf "dist/GodseonSync-${VERSION}-linux-${arch}.tar.gz" GodseonSync
    echo "→ dist/GodseonSync-${VERSION}-linux-${arch}.tar.gz"
  done
}

case "${1:-}" in
  mac) build_mac ;;
  windows) build_windows ;;
  windows-zip) zip_windows ;;
  linux) build_linux ;;
  *) echo "사용법: $0 mac|windows|windows-zip|linux" >&2; exit 2 ;;
esac
