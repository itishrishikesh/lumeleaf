#!/bin/sh
set -eu

: "${VERSION:?VERSION is required}"
: "${BINARY:?BINARY is required}"

arch=${ARCH:-$(uname -m)}
dist=${DIST:-dist}
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/lumeleaf-macos-package.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM

mkdir -p "$dist"
app="$stage/Lumeleaf.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
install -m 0755 "$BINARY" "$app/Contents/MacOS/Lumeleaf"
sed "s|@VERSION@|$VERSION|g" "$repo_root/packaging/macos/Info.plist.in" > "$app/Contents/Info.plist"
install -m 0644 "$repo_root/LICENSE" "$repo_root/THIRD_PARTY_NOTICES.md" "$app/Contents/Resources/"
printf 'APPL????' > "$app/Contents/PkgInfo"
codesign --force --deep --sign - "$app"

ditto -c -k --sequesterRsrc --keepParent "$app" "$dist/Lumeleaf-${VERSION}-macos-${arch}.app.zip"
dmgroot="$stage/dmg"
mkdir -p "$dmgroot"
ditto "$app" "$dmgroot/Lumeleaf.app"
ln -s /Applications "$dmgroot/Applications"
hdiutil create -quiet -ov -format UDZO -volname "Lumeleaf $VERSION" -srcfolder "$dmgroot" "$dist/Lumeleaf-${VERSION}-macos-${arch}.dmg"
install -m 0755 "$BINARY" "$dist/lumeleaf-macos-${arch}"
