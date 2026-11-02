#!/bin/sh
# Installs Portero.app into ~/Applications, links the CLI into ~/.local/bin and leaves exactly one copy
# registered with Launch Services: every other registered Portero.app is unregistered.
# Does NOT change the default browser: run `portero default set`.
# INSTALL_DRY_RUN=1 builds in a temp directory and only prints what would happen.
set -eu
cd "$(dirname "$0")/.."
LSR=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
ID=dev.plavipas.portero
DEST="$HOME/Applications/Portero.app"
DRY=${INSTALL_DRY_RUN:-}

# Capture first: a pipe into tail would hide a failed build and an empty APP would reach rm -rf "$DEST".
built=$(sh scripts/build-app.sh) || { echo "error: build failed; nothing was changed" >&2; exit 1; }
APP=$(printf '%s\n' "$built" | tail -1)
if [ -z "$APP" ] || [ ! -d "$APP/Contents/MacOS" ]; then
	echo "error: build produced no app ($APP); nothing was changed" >&2
	exit 1
fi
BUILD_DIR=$(dirname "$APP")

# unreg PATH: lsregister -u with a 10 s watchdog; a hang or failure only warns.
unreg() {
	"$LSR" -u "$1" >/dev/null 2>&1 &
	pid=$!
	( sleep 10; kill "$pid" 2>/dev/null ) >/dev/null 2>&1 &
	wd=$!
	if wait "$pid" 2>/dev/null; then :; else echo "warning: could not unregister $1" >&2; fi
	kill "$wd" 2>/dev/null || true
	wait "$wd" 2>/dev/null || true
}

# rmbuild: remove the temporary build directory, but only when build-app.sh created it with mktemp (no
# PORTERO_BUILD_DIR given): a directory the caller chose is never deleted, whatever its name.
rmbuild() {
	[ -z "${PORTERO_BUILD_DIR:-}" ] || return 0
	case "$BUILD_DIR" in
	*/portero-build.*) rm -rf "$BUILD_DIR" ;;
	esac
}

others() { "$LSR" -dump | "$APP/Contents/MacOS/portero" lsreg-others "$ID" "$DEST"; }

if [ -n "$DRY" ]; then
	echo "would copy $APP to $DEST"
	others | while IFS= read -r p; do
		case "$p" in /*.app) echo "would unregister $p" ;; esac
	done
	echo "would register $DEST"
	echo "would link $HOME/.local/bin/portero"
	[ -n "${PORTERO_BUILD_DIR:-}" ] || echo "would remove $BUILD_DIR"
	rmbuild
	exit 0
fi

mkdir -p "$HOME/Applications" "$HOME/.local/bin"
rm -rf "$DEST"
cp -R "$APP" "$DEST"
# The sweep uses the freshly installed binary; the dump may list the build copy too, it is already gone.
others_list=$("$LSR" -dump | "$DEST/Contents/MacOS/portero" lsreg-others "$ID" "$DEST" || true)
printf '%s\n' "$others_list" | while IFS= read -r p; do
	case "$p" in /*.app) echo "unregistering $p"; unreg "$p" ;; esac
done
rmbuild
"$LSR" -f "$DEST"
ln -sf "$DEST/Contents/MacOS/portero" "$HOME/.local/bin/portero"
mdfind "kMDItemCFBundleIdentifier == '$ID'" 2>/dev/null | while IFS= read -r p; do
	[ -n "$p" ] && [ "$p" != "$DEST" ] && echo "stale copy on disk (not deleted): $p"
done || true
echo "installed $DEST; CLI: ~/.local/bin/portero"
"$DEST/Contents/MacOS/portero" default status
