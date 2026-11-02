# Linux support: task for a contributor on a Linux desktop

Status: open, not started. Nobody on the core team runs a Linux desktop, so this task is meant to be picked up and
built end to end by someone who does: you can run Portero live on GNOME or KDE; we can't.

## Context you need first

- Go 1.27, standard library plus `gopkg.in/yaml.v3`. Read `README.md`, `docs/rules.md`, `docs/ui.md`, `docs/adr/`
  and `CLAUDE.md` (the working rules and the agent workflow of this repository).
- The core is already portable and builds on Linux today: `internal/rules` (format, groups, matching on
  host/path/opener/cwd), `internal/config`, `internal/decisionlog`, `internal/check`, `internal/serve` (the local
  web UI), the `cmd/portero` CLI. `GOOS=linux go build ./...` and `go test ./...` pass.
- macOS-only today, behind build tags: `internal/handler` (cgo/Objective-C Apple-event URL handler and the
  default-browser setter), `internal/launch/launch_darwin.go` (`open -a/-b`, AppleScript for Dia/Arc),
  `internal/browsers/installed.go` (looks in /Applications), `cmd/portero/serve_darwin.go`, `cmd/portero/app.go`
  (the .app bundle). The `_other` files return "not supported".

## Goal

On a Linux desktop Portero is the default browser: every http/https link from a chat, a mail client, a terminal
or `$BROWSER` goes through the rules and opens in the browser and profile the rules pick; `decisions.log` gets a
line per link.

## What to build

1. **Registration.** A `portero.desktop` entry (`Exec=portero open %u`,
   `MimeType=x-scheme-handler/http;x-scheme-handler/https;text/html;`, `NoDisplay=true`) installed to
   `~/.local/share/applications`. `portero default status|set` on Linux via
   `xdg-settings get/set default-web-browser portero.desktop` (fall back to `xdg-mime default` per scheme);
   `status` shows the handler per scheme like on macOS.
2. **Handler.** No app bundle and no Apple events: the desktop entry runs the CLI. `opener` is unknown on Linux
   (leave it empty and document that opener rules never match there); `cwd` rules work for links opened from a
   shell, as on macOS.
3. **Launchers** (`internal/launch/launch_linux.go`):
   - Chromium family (google-chrome, chromium, brave-browser, vivaldi, microsoft-edge):
     `<binary> --profile-directory=<dir> <url>`; profile display names from `~/.config/<browser>/Local State`
     (same parser as on macOS, different base directory).
   - Firefox family (firefox, librewolf): `firefox -P <name> <url>` from `~/.mozilla/firefox/profiles.ini`
     (`[Install…] Default=` first, as on macOS); containers through the same `ext+container:` URL when the
     "Open external links in a container" extension is active (reuse `internal/browsers/containers.go` and
     `internal/launch/container.go`). Check what happens when the profile is already running (`-P` vs
     `--new-tab`) and keep the fail-safe: no extension, plain URL.
   - Anything else: run the browser's binary directly. Never call `xdg-open` for a URL: it would loop back into
     Portero.
   - Dia, Arc, Safari and the macOS-only browsers do not exist on Linux: `rules check` warns "not available on
     Linux" and `open` falls back to the default target with a note.
   - Never pass URLs or names through a shell: `exec.Command` with argv only (see the macOS code for the pattern
     and the `ext+container` encoder).
4. **`portero browsers`** on Linux: installed browsers from PATH and desktop entries, with profiles and
   containers.
5. **`portero serve`** works as is. In `serve_linux.go` wire the Settings default-browser button to the xdg path
   or leave it unsupported (501, hidden button); document the choice. iCloud sync is macOS-only: hide it on Linux.
6. **Install.** `make install-linux`: binary to `~/.local/bin/portero`, the desktop entry,
   `update-desktop-database ~/.local/share/applications`. No root.
7. **CI.** A GitHub Actions job on `ubuntu-latest` running `go vet ./... && go test ./...` (pin actions to commit
   SHAs like the existing workflows).
8. **Docs.** A platform section in README (macOS / Linux), notes in `docs/rules.md` (opener is macOS-only;
   profiles per platform), a Linux install section.

## Acceptance

- On GNOME and/or KDE (say which): `portero default set` makes Portero the handler for http/https
  (`xdg-settings get default-web-browser` prints `portero.desktop`); a link clicked in a terminal app and one
  from a chat app open in the browser and profile the rules pick; `decisions.log` gets the lines.
- `portero open <url> --dry-run` prints the exact argv for each launcher family; unit tests cover the argv
  building and the profile/container parsing with fixtures (no real browser needed in CI).
- macOS behaviour unchanged; `make test` green on macOS and in the Linux CI job; `GOOS=darwin go build ./...`
  still builds.
- No personal hosts, profile names or paths in the repository (placeholders Personal / Work, example.com).

## How to work

Branch `linux`, small commits, a pull request into `main`; the pull request gets the automatic Claude Code
review. Keep the macOS code paths untouched unless a shared helper has to move; if a decision in `docs/adr/` has
to change, say so in the pull request.
