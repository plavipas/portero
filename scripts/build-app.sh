#!/bin/sh
# Builds build/Portero.app from the Go binary and packaging/Info.plist (no Xcode needed).
set -eu
cd "$(dirname "$0")/.."
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
APP=build/Portero.app
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp packaging/Portero.icns "$APP/Contents/Resources/Portero.icns"
CGO_ENABLED=1 go build -ldflags "-X main.version=$VERSION" -o "$APP/Contents/MacOS/portero" ./cmd/portero
sed "s/VERSION/$VERSION/g" packaging/Info.plist > "$APP/Contents/Info.plist"
printf 'APPL????' > "$APP/Contents/PkgInfo"
codesign --force --sign - "$APP" >/dev/null 2>&1 || echo "warning: ad-hoc codesign failed" >&2
echo "$APP ($VERSION)"
