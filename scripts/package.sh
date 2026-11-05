#!/bin/sh
# Builds the universal Portero.app, checks it and packs dist/Portero_<version>_darwin_universal.zip plus
# dist/checksums.txt. The app is built in a temporary directory outside the repository (Launch Services must never
# index a stray copy). VERSION works as in build-app.sh (a leading v is dropped).
set -eu
cd "$(dirname "$0")/.."
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
VERSION=${VERSION#v}
export VERSION
BUILD=$(mktemp -d "${TMPDIR:-/tmp}/portero-package.XXXXXX")
trap 'rm -rf "$BUILD"' EXIT
out=$(PORTERO_ARCHS="arm64 x86_64" PORTERO_BUILD_DIR="$BUILD" sh scripts/build-app.sh) || {
	echo "error: build failed" >&2
	exit 1
}
APP=$(printf '%s\n' "$out" | tail -n 1)
[ -d "$APP" ] || { echo "error: build did not produce an app ($APP)" >&2; exit 1; }
codesign --verify --strict "$APP"
archs=$(lipo -archs "$APP/Contents/MacOS/portero")
case " $archs " in
*" arm64 "*) ;;
*) echo "error: binary lacks arm64 ($archs)" >&2; exit 1 ;;
esac
case " $archs " in
*" x86_64 "*) ;;
*) echo "error: binary lacks x86_64 ($archs)" >&2; exit 1 ;;
esac
plist=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")
bin=$("$APP/Contents/MacOS/portero" version)
if [ "$bin" != "portero $plist" ] || [ "$plist" != "$VERSION" ]; then
	echo "error: version mismatch: Info.plist '$plist', binary '$bin', expected '$VERSION'" >&2
	exit 1
fi
mkdir -p dist
ZIP=Portero_${VERSION}_darwin_universal.zip
rm -f "dist/$ZIP"
ditto -c -k --norsrc --noextattr --keepParent "$APP" "dist/$ZIP"
# The archive must unpack with plain tools into a bundle whose signature still verifies.
CHECK=$BUILD/unzip
mkdir -p "$CHECK"
unzip -q "dist/$ZIP" -d "$CHECK"
codesign --verify --strict "$CHECK/Portero.app" || { echo "error: signature broken after unzip" >&2; exit 1; }
(cd dist && shasum -a 256 "$ZIP" > checksums.txt)
echo "packaged dist/$ZIP" >&2
cat dist/checksums.txt >&2
