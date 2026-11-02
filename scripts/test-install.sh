#!/bin/sh
# Checks that a failing build leaves the installed app intact (fake HOME, nothing real is touched).
set -eu
cd "$(dirname "$0")/.."
T=$(mktemp -d "${TMPDIR:-/tmp}/portero-installtest.XXXXXX")
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/Applications/Portero.app"
echo keep > "$T/Applications/Portero.app/marker"
for dry in "" 1; do
	if HOME=$T INSTALL_DRY_RUN=$dry PORTERO_BUILD_DIR="$PWD/build" sh scripts/install.sh >/dev/null 2>&1; then
		echo "FAIL: install.sh succeeded with a build directory inside the repo (dry=$dry)" >&2
		exit 1
	fi
	[ "$(cat "$T/Applications/Portero.app/marker")" = keep ] || { echo "FAIL: installed app was touched (dry=$dry)" >&2; exit 1; }
done
[ ! -e build ] || { echo "FAIL: build/ left in the repo" >&2; exit 1; }
echo "ok: failing build leaves the installed app intact"

# A build directory the caller chose is never deleted, even when its name looks like a temp one.
B="$T/portero-build.mine"
mkdir -p "$B"
echo keep > "$B/marker"
HOME=$T INSTALL_DRY_RUN=1 PORTERO_BUILD_DIR="$B" sh scripts/install.sh >/dev/null 2>&1 || { echo "FAIL: dry-run install failed" >&2; exit 1; }
[ "$(cat "$B/marker" 2>/dev/null)" = keep ] || { echo "FAIL: caller's build directory was removed" >&2; exit 1; }
echo "ok: a caller-provided build directory is kept"
