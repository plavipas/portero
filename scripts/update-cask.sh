#!/bin/sh
# Renders Casks/portero.rb of the Homebrew tap for a published release.
# Usage: scripts/update-cask.sh <tag> <tap-dir>   (for example: v0.1.0 ~/src/homebrew-tap)
# It downloads checksums.txt of the release with gh, takes the sha256 of the zip and writes
# <tap-dir>/Casks/portero.rb from packaging/portero.rb.tmpl. It runs no git commands: review and commit in the tap.
set -eu
REPO=$(cd "$(dirname "$0")/.." && pwd -P)
if [ $# -ne 2 ]; then
	echo "usage: $0 <tag> <tap-dir>" >&2
	exit 2
fi
TAG=$1
TAPARG=$2
VERSION=${TAG#v}
case "$TAG" in
v[0-9]*) ;;
"") echo "error: empty tag" >&2; exit 1 ;;
*) echo "error: tag $TAG is not of the form vMAJOR.MINOR.PATCH" >&2; exit 1 ;;
esac
case "$VERSION" in
*[!0-9A-Za-z.+-]*)
	echo "error: version contains characters other than letters, digits and . + -" >&2
	exit 1
	;;
esac
# Whole-string match without grep (a newline cannot hide a second line): three numeric parts.
case "$VERSION" in
*.*.*) ;;
*) echo "error: version $VERSION is not MAJOR.MINOR.PATCH" >&2; exit 1 ;;
esac
core=${VERSION%%[-+]*}
major=${core%%.*}; rest=${core#*.}; minor=${rest%%.*}; patch=${rest#*.}
for n in "$major" "$minor" "$patch"; do
	case "$n" in
	"" | *[!0-9]*) echo "error: version $VERSION is not MAJOR.MINOR.PATCH" >&2; exit 1 ;;
	esac
done
# Resolve the tap before any cd, so a relative path means relative to the caller's directory.
TAP=$(cd "$TAPARG" 2>/dev/null && pwd -P) || { echo "error: tap directory $TAPARG does not exist" >&2; exit 1; }
case "$TAP/" in
"$REPO"/*) echo "error: tap directory $TAP is inside the repository $REPO" >&2; exit 1 ;;
esac
cd "$REPO"
WORK=$(mktemp -d "${TMPDIR:-/tmp}/portero-cask.XXXXXX")
trap 'rm -rf "$WORK"' EXIT
if [ -n "${PORTERO_CHECKSUMS:-}" ]; then
	# Tests: a local checksums file instead of the release download.
	cp "$PORTERO_CHECKSUMS" "$WORK/checksums.txt"
else
	gh release download "$TAG" --repo plavipas/portero --pattern checksums.txt --dir "$WORK"
fi
ZIP=Portero_${VERSION}_darwin_universal.zip
count=$(awk -v z="$ZIP" '$2 == z' "$WORK/checksums.txt" | wc -l | tr -d ' ')
if [ "$count" != 1 ]; then
	echo "error: expected exactly one checksum line for $ZIP, found $count" >&2
	exit 1
fi
SHA=$(awk -v z="$ZIP" '$2 == z { print $1 }' "$WORK/checksums.txt")
if ! printf '%s' "$SHA" | grep -Eq '^[0-9a-f]{64}$'; then
	echo "error: no valid sha256 for $ZIP in checksums.txt" >&2
	exit 1
fi
sed -e 's/^\(.\)/    \1/' packaging/portero-shim.sh > "$WORK/shim.indented"
sed -e "s/@VERSION@/$VERSION/" -e "s/@SHA256@/$SHA/" -e "/@SHIM@/r $WORK/shim.indented" -e "/@SHIM@/d" packaging/portero.rb.tmpl > "$WORK/portero.rb"
if grep -Eq '@VERSION@|@SHA256@|@SHIM@' "$WORK/portero.rb"; then
	echo "error: a placeholder is left in the rendered cask" >&2
	exit 1
fi
if command -v ruby >/dev/null 2>&1; then
	ruby -c "$WORK/portero.rb" >/dev/null || { echo "error: the rendered cask is not valid Ruby" >&2; exit 1; }
fi
mkdir -p "$TAP/Casks"
mv "$WORK/portero.rb" "$TAP/Casks/portero.rb"
echo "wrote $TAP/Casks/portero.rb ($VERSION, $SHA)" >&2
