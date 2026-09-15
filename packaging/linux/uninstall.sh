#!/bin/sh
set -eu

prefix=${LUMELEAF_PREFIX:-"$HOME/.local"}
rm -f "$prefix/bin/lumeleaf"
rm -f "$prefix/share/applications/io.github.itishrishikesh.Lumeleaf.desktop"
rm -f "$prefix/share/icons/hicolor/scalable/apps/io.github.itishrishikesh.Lumeleaf.svg"
rm -f "$prefix/share/metainfo/io.github.itishrishikesh.Lumeleaf.metainfo.xml"
printf 'Lumeleaf removed from %s\n' "$prefix"
