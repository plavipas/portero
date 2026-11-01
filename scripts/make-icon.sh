#!/bin/sh
# Regenerates packaging/Portero.icns from docs/logo.svg using only system tools
# (swift from the Command Line Tools, sips, iconutil). Run manually only when the logo changes; it is not
# part of the build. Needs a GUI-capable session (AppKit).
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

MASTER="$TMP/master.png"
swift scripts/render-icon.swift "$TMP/padded.svg" "$MASTER" 1024

# The corner outside the rounded tile must be fully transparent.
cat > "$TMP/check.swift" <<'SW'
import AppKit
let rep = NSBitmapImageRep(data: try! Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1])))!
let a = rep.colorAt(x: 0, y: 0)!.alphaComponent
let c = rep.colorAt(x: 512, y: 512)!.alphaComponent
if rep.pixelsWide != 1024 || a != 0 || c < 0.99 { print("bad alpha: corner \(a) centre \(c)"); exit(1) }
SW
swift "$TMP/check.swift" "$MASTER" || { echo "make-icon: master PNG is not transparent outside the tile" >&2; exit 1; }

SET="$TMP/Portero.iconset"
mkdir "$SET"
for spec in 16:icon_16x16 32:icon_16x16@2x 32:icon_32x32 64:icon_32x32@2x \
	128:icon_128x128 256:icon_128x128@2x 256:icon_256x256 512:icon_256x256@2x \
	512:icon_512x512 1024:icon_512x512@2x; do
	n=${spec%%:*}
	name=${spec#*:}
	sips -z "$n" "$n" "$MASTER" --out "$SET/$name.png" >/dev/null
done

iconutil -c icns "$SET" -o packaging/Portero.icns
/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' packaging/Info.plist | grep -qx 13.0 \
	|| { echo "make-icon: LSMinimumSystemVersion is not 13.0" >&2; exit 1; }
echo "packaging/Portero.icns"
