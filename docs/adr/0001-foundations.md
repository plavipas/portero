# ADR 0001: Foundations of Portero

Status: accepted, 2026-11-01.

## Decisions

1. **Go** with a portable core (`internal/rules`, `browsers`, `launch` plan, `config`, `decisionlog`, `sync`)
   and the macOS-only parts behind `//go:build darwin`: the Apple event handler and the default-browser calls
   are cgo/Objective-C in `internal/handler`, launching goes through `/usr/bin/open` and `/usr/bin/osascript`.
   Dependencies: standard library plus `gopkg.in/yaml.v3`.
2. **Rules are one local YAML file** (`docs/rules.md`), edited by hand in v1; sharing between Macs is done by
   putting the file into a synced folder (iCloud Drive in v1, `portero sync icloud`). No service, no account.
3. **Single-instance browsers** (Dia, Arc) that ignore command-line profile flags are driven through their
   AppleScript dictionaries; Chromium and Firefox families through their command-line profile flags; anything
   else through plain `open`. Data (URL, profile) reaches AppleScript only through `argv`.
4. **A link is never lost**: when the rules file is missing or invalid the link opens in Safari and the error
   is logged. Every routed link appends one line to `decisions.log`.
5. **The handler app** (`Portero.app`, `LSUIElement`) is started by Launch Services per link, routes on a
   goroutine, replies to the Apple event at once and quits two seconds after the last link.
6. **Nothing personal in the repository**: no real hostnames, profile names, workspace names or bundle ids of
   private tools; examples use `example.com`-style hosts and the profiles Personal / Work. The tool is
   described for any user with any browsers.
7. **Private until the first working release**; license to be settled at release.

## Consequences

- Other operating systems compile (`launch_other.go`, `handler_other.go`) but do nothing yet.
- Arc support is written from its dictionary and unverified; Arc is not a priority.
- Links clicked inside a browser stay there; a companion extension is a later feature (Linear DEV-21).
