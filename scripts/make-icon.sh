#!/bin/sh
# Regenerates packaging/Portero.icns from docs/logo.svg using only system tools
# (qlmanage, sips, iconutil). Run manually only when the logo changes; it is not
# part of the build. qlmanage needs a GUI session.
set -eu
cd "$(dirname "$0")/.."
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# Apple HIG: the tile is 824 px on a 1024 px canvas, transparent 100 px margins.
{
	printf '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">\n'
	sed '1,/<svg /s/<svg /<svg x="100" y="100" width="824" height="824" /' docs/logo.svg
	printf '\n</svg>\n'
} > "$TMP/padded.svg"

qlmanage -t -s 1024 -o "$TMP" "$TMP/padded.svg" >/dev/null 2>&1 || true
if [ ! -f "$TMP/padded.svg.png" ]; then
	echo "make-icon: qlmanage produced no PNG (it needs a GUI session)" >&2
	exit 1
fi

SET="$TMP/Portero.iconset"
mkdir "$SET"
for spec in 16:icon_16x16 32:icon_16x16@2x 32:icon_32x32 64:icon_32x32@2x \
	128:icon_128x128 256:icon_128x128@2x 256:icon_256x256 512:icon_256x256@2x \
	512:icon_512x512 1024:icon_512x512@2x; do
	n=${spec%%:*}
	name=${spec#*:}
	sips -z "$n" "$n" "$TMP/padded.svg.png" --out "$SET/$name.png" >/dev/null
done

iconutil -c icns "$SET" -o packaging/Portero.icns
echo "packaging/Portero.icns"
