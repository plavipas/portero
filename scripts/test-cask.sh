#!/bin/sh
# Tests the cask CLI shim and scripts/update-cask.sh in temp dirs. Installs nothing, runs no brew.
set -eu
cd "$(dirname "$0")/.."
REPO=$PWD
T=$(mktemp -d "${TMPDIR:-/tmp}/portero-casktest.XXXXXX")
trap 'rm -rf "$T"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

# --- the shim, with #{appdir} replaced by a temp dir and a fake binary that prints its argv
APPDIR=$T/appdir
APP=$APPDIR/Portero.app
mkdir -p "$APP/Contents/MacOS"
sed "s|#{appdir}|$APPDIR|" packaging/portero-shim.sh > "$T/shim.sh"
run_shim() { sh "$T/shim.sh" "$@"; }

rc=0; run_shim >/dev/null 2>"$T/err" || rc=$?
[ "$rc" = 1 ] || fail "missing binary: exit $rc, want 1"
grep -q 'brew reinstall --cask portero' "$T/err" || fail "missing binary: no brew reinstall hint"

printf '#!/bin/sh\nfor a in "$@"; do printf "[%%s]" "$a"; done\n' > "$APP/Contents/MacOS/portero"
chmod +x "$APP/Contents/MacOS/portero"

out=$(run_shim open "a b" "it's") || fail "no quarantine: exit $?"
[ "$out" = "[open][a b][it's]" ] || fail "no quarantine: argv changed: $out"
echo "ok: shim without quarantine execs the binary with argv intact"

if [ "$(uname -s)" = Darwin ]; then
	setq() { /usr/bin/xattr -w com.apple.quarantine "$1" "$APP"; }
	setq "0081;0;Safari;"
	rc=0; run_shim open x >"$T/out" 2>"$T/err" || rc=$?
	[ "$rc" = 3 ] || fail "flags 0081: exit $rc, want 3"
	[ ! -s "$T/out" ] || fail "flags 0081: the binary ran"
	grep -q 'Open Anyway' "$T/err" || fail "flags 0081: no Open Anyway hint"
	grep -q 'xattr -dr com.apple.quarantine' "$T/err" || fail "flags 0081: no xattr hint"
	for f in "00c1;0;Safari;" "zz;0;Safari;"; do
		setq "$f"
		out=$(run_shim go) || fail "flags $f: exit $?"
		[ "$out" = "[go]" ] || fail "flags $f: did not exec: $out"
	done
	echo "ok: shim blocks an unapproved app (exit 3) and execs an approved or unknown one"
else
	echo "skip: quarantine cases run on macOS only"
fi

# --- update-cask.sh from another cwd with a relative tap
V=9.9.9
SHA=$(printf 'x' | shasum -a 256 | cut -d' ' -f1)
echo "$SHA  Portero_${V}_darwin_universal.zip" > "$T/checksums.txt"
mkdir -p "$T/work/tap"
had=0; [ ! -e Casks ] || had=1
(cd "$T/work" && PORTERO_CHECKSUMS="$T/checksums.txt" sh "$REPO/scripts/update-cask.sh" "v$V" ./tap) 2>"$T/err" || { cat "$T/err" >&2; fail "update-cask.sh failed"; }
C=$T/work/tap/Casks/portero.rb
[ -f "$C" ] || fail "cask not written to the tap"
[ "$had" = 1 ] || [ ! -e Casks ] || fail "Casks/ appeared in the repo"
if grep -Eq '@VERSION@|@SHA256@|@SHIM@' "$C"; then fail "placeholder left"; fi
for want in 'command_wrapper "portero"' 'preflight_steps' 'if_path_exists "Applications/Portero.app", base: :home' 'Open Anyway' "version \"$V\"" "sha256 \"$SHA\""; do
	grep -qF "$want" "$C" || fail "cask lacks: $want"
done
if command -v ruby >/dev/null 2>&1; then ruby -c "$C" >/dev/null || fail "ruby -c"; fi
echo "ok: update-cask.sh renders into a relative tap"

# A tap inside the repository is refused and nothing is written.
rc=0; PORTERO_CHECKSUMS="$T/checksums.txt" sh scripts/update-cask.sh "v$V" . >/dev/null 2>&1 || rc=$?
[ "$rc" != 0 ] || fail "tap inside the repo was accepted"
[ "$had" = 1 ] || [ ! -e Casks ] || fail "Casks/ appeared in the repo after the refused run"
echo "ok: a tap inside the repository is refused"

# The rendered cask must pass brew style (read-only); skipped without brew.
if command -v brew >/dev/null 2>&1; then
	HOMEBREW_NO_AUTO_UPDATE=1 brew style "$C" >"$T/style" 2>&1 || { cat "$T/style" >&2; fail "brew style reports offenses"; }
	echo "ok: the rendered cask passes brew style"
else
	echo "skip: brew style needs Homebrew"
fi
