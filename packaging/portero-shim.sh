#!/bin/sh
# Installed as the "portero" command by the Homebrew cask (command_wrapper). It is pasted into a Ruby heredoc:
# keep it free of backslashes and of the Ruby interpolation marker except for the appdir below.
APP="#{appdir}/Portero.app"
BIN="$APP/Contents/MacOS/portero"
if [ ! -x "$BIN" ]; then
  echo "Portero.app not found at $APP; run: brew reinstall --cask portero" >&2
  exit 1
fi
# A quarantined, not yet approved app makes Gatekeeper hang the first launch; print a hint instead.
FLAGS=$(/usr/bin/xattr -p com.apple.quarantine "$APP" 2>/dev/null) || exec "$BIN" "$@"
FLAGS=${FLAGS%%;*}
case "$FLAGS" in
  "" | *[!0-9a-fA-F]*) exec "$BIN" "$@" ;;
esac
# Bit 0x40 is set by macOS once the user approved the app.
if [ $((0x$FLAGS & 64)) -ne 0 ]; then
  exec "$BIN" "$@"
fi
echo "portero: $APP is quarantined and not approved yet, so macOS would block it." >&2
echo "  Approve it: open -a Portero, then System Settings > Privacy & Security > Open Anyway." >&2
echo "  Or run: xattr -dr com.apple.quarantine $APP" >&2
exit 3
