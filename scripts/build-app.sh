#!/bin/sh
# Builds Portero.app from the Go binary and packaging/Info.plist (no Xcode needed).
# The app is built OUTSIDE the repository so Launch Services never indexes a stray copy: into
# $PORTERO_BUILD_DIR when set, else into a fresh mktemp directory under $TMPDIR. The last line on stdout is
# the absolute path of the .app; the version goes to stderr. A build directory inside the repository is refused.
set -eu
cd "$(dirname "$0")/.."
ROOT=$(pwd -P)
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
# The version goes into sed and an XML plist: only plain version characters are accepted.
case "$VERSION" in
*[!A-Za-z0-9._+-]* | "")
	echo "error: version $VERSION may only contain letters, digits and . _ + -" >&2
	exit 1
	;;
esac
OUT=${PORTERO_BUILD_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/portero-build.XXXXXX")}
case "$OUT" in /*) ;; *) OUT=$PWD/$OUT ;; esac
case "$OUT/" in
"$ROOT"/*)
	echo "error: build directory $OUT is inside the repository; a .app there gets registered with Launch Services" >&2
	exit 1
	;;
esac
existed=0
[ -d "$OUT" ] && existed=1
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd -P)
# Never build over the installed app: install would then delete its own build.
APPS=$(cd "$HOME/Applications" 2>/dev/null && pwd -P || true)
if [ -n "$APPS" ] && { [ "$OUT" = "$APPS" ] || case "$OUT/" in "$APPS"/*) true ;; *) false ;; esac; }; then
	[ "$existed" = 1 ] || rmdir "$OUT" 2>/dev/null || true
	echo "error: build directory $OUT is inside ~/Applications; build elsewhere and let make install copy it" >&2
	exit 1
fi
# Again on the resolved path: a symlink outside the repository may point inside it.
case "$OUT/" in
"$ROOT"/*)
	[ "$existed" = 1 ] || rmdir "$OUT" 2>/dev/null || true
	echo "error: build directory $OUT is inside the repository; a .app there gets registered with Launch Services" >&2
	exit 1
	;;
esac
APP=$OUT/Portero.app
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp packaging/Portero.icns "$APP/Contents/Resources/Portero.icns"
CGO_ENABLED=1 go build -ldflags "-X main.version=$VERSION" -o "$APP/Contents/MacOS/portero" ./cmd/portero
sed "s/VERSION/$VERSION/g" packaging/Info.plist > "$APP/Contents/Info.plist"
printf 'APPL????' > "$APP/Contents/PkgInfo"
codesign --force --sign - "$APP" >/dev/null 2>&1 || echo "warning: ad-hoc codesign failed" >&2
echo "built Portero $VERSION" >&2
echo "$APP"
