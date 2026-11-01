#!/bin/sh
# Installs build/Portero.app into ~/Applications, links the CLI into ~/.local/bin and registers
# the app with Launch Services. Does NOT change the default browser: run `portero default set`.
set -eu
cd "$(dirname "$0")/.."
sh scripts/build-app.sh
DEST="$HOME/Applications/Portero.app"
mkdir -p "$HOME/Applications" "$HOME/.local/bin"
rm -rf "$DEST"
cp -R build/Portero.app "$DEST"
ln -sf "$DEST/Contents/MacOS/portero" "$HOME/.local/bin/portero"
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$DEST"
echo "installed $DEST; CLI: ~/.local/bin/portero"
"$DEST/Contents/MacOS/portero" default status
