#!/bin/sh
set -eu

bundle_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix=${LUMELEAF_PREFIX:-"$HOME/.local"}

install -d "$prefix/bin" "$prefix/share/applications" "$prefix/share/icons/hicolor/scalable/apps" "$prefix/share/metainfo"
install -m 0755 "$bundle_dir/lumeleaf" "$prefix/bin/lumeleaf"
sed "s|@EXEC@|$prefix/bin/lumeleaf|g" "$bundle_dir/resources/io.github.itishrishikesh.Lumeleaf.desktop.in" > "$prefix/share/applications/io.github.itishrishikesh.Lumeleaf.desktop"
install -m 0644 "$bundle_dir/resources/io.github.itishrishikesh.Lumeleaf.svg" "$prefix/share/icons/hicolor/scalable/apps/io.github.itishrishikesh.Lumeleaf.svg"
install -m 0644 "$bundle_dir/resources/io.github.itishrishikesh.Lumeleaf.metainfo.xml" "$prefix/share/metainfo/io.github.itishrishikesh.Lumeleaf.metainfo.xml"

printf 'Lumeleaf installed to %s\n' "$prefix"
printf 'Run %s/bin/lumeleaf [FILE]\n' "$prefix"
