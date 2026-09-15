#!/bin/sh
set -eu

: "${VERSION:?VERSION is required}"
: "${BINARY:?BINARY is required}"

arch=${ARCH:-amd64}
dist=${DIST:-dist}
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stage=$(mktemp -d "${TMPDIR:-/tmp}/lumeleaf-linux-package.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM

mkdir -p "$dist"
bundle_name="lumeleaf-${VERSION}-linux-${arch}"
bundle="$stage/$bundle_name"
mkdir -p "$bundle/resources"
install -m 0755 "$BINARY" "$bundle/lumeleaf"
install -m 0755 "$repo_root/packaging/linux/install.sh" "$bundle/install.sh"
install -m 0755 "$repo_root/packaging/linux/uninstall.sh" "$bundle/uninstall.sh"
install -m 0644 "$repo_root/packaging/linux/lumeleaf.desktop.in" "$bundle/resources/io.github.itishrishikesh.Lumeleaf.desktop.in"
install -m 0644 "$repo_root/packaging/linux/io.github.itishrishikesh.Lumeleaf.metainfo.xml" "$bundle/resources/io.github.itishrishikesh.Lumeleaf.metainfo.xml"
install -m 0644 "$repo_root/assets/icon.svg" "$bundle/resources/io.github.itishrishikesh.Lumeleaf.svg"
install -m 0644 "$repo_root/README.md" "$repo_root/LICENSE" "$repo_root/THIRD_PARTY_NOTICES.md" "$bundle/"
tar -C "$stage" -czf "$dist/${bundle_name}.tar.gz" "$bundle_name"
install -m 0755 "$BINARY" "$dist/lumeleaf-linux-${arch}"

if command -v dpkg-deb >/dev/null 2>&1; then
	debroot="$stage/deb"
	mkdir -p "$debroot/DEBIAN" "$debroot/usr/bin" "$debroot/usr/share/applications" "$debroot/usr/share/icons/hicolor/scalable/apps" "$debroot/usr/share/metainfo" "$debroot/usr/share/doc/lumeleaf"
	install -m 0755 "$BINARY" "$debroot/usr/bin/lumeleaf"
	sed 's|@EXEC@|/usr/bin/lumeleaf|g' "$repo_root/packaging/linux/lumeleaf.desktop.in" > "$debroot/usr/share/applications/io.github.itishrishikesh.Lumeleaf.desktop"
	install -m 0644 "$repo_root/assets/icon.svg" "$debroot/usr/share/icons/hicolor/scalable/apps/io.github.itishrishikesh.Lumeleaf.svg"
	install -m 0644 "$repo_root/packaging/linux/io.github.itishrishikesh.Lumeleaf.metainfo.xml" "$debroot/usr/share/metainfo/io.github.itishrishikesh.Lumeleaf.metainfo.xml"
	install -m 0644 "$repo_root/LICENSE" "$repo_root/THIRD_PARTY_NOTICES.md" "$debroot/usr/share/doc/lumeleaf/"
	printf '%s\n' \
		'Package: lumeleaf' \
		"Version: $VERSION" \
		'Section: devel' \
		'Priority: optional' \
		'Architecture: amd64' \
		'Maintainer: Lumeleaf contributors' \
		'Depends: libc6, libx11-6, libx11-xcb1, libwayland-client0, libxkbcommon0, libegl1, libgles2, libxcursor1' \
		'Description: Fast native Java and Markdown reader' \
		' Lumeleaf is a local-first, view-first workspace for reviewing code.' > "$debroot/DEBIAN/control"
	dpkg-deb --root-owner-group --build "$debroot" "$dist/lumeleaf_${VERSION}_amd64.deb"
fi
