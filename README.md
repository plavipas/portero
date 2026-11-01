# Portero

Portero is a default-browser router for macOS. It registers itself as the handler for `http` and
`https` links and opens every link that comes from another app (chat, mail, calendar, terminal,
scripts, `$BROWSER`) in the browser and profile that a rule picks. One link, one place, every time.

## What it does

- Routes by host (wildcards), by path prefix and by the app that opened the link (bundle id).
- Targets any installed browser and, where the browser has them, a specific profile, space or
  container.
- Supports the new generation of browsers as well as the classic ones. Browsers that run as a single
  instance and ignore command-line arguments (Dia, Arc and the like) are driven through their
  AppleScript dictionary, so a profile can still be chosen.
- Keeps the rules in one plain file on the Mac; the file can live in iCloud Drive to be shared between
  Macs.
- Logs every routing decision.
- Ships a CLI: `portero open <url>`, `portero rules`, `portero sync`, usable as `$BROWSER`.

## Browser support

| Browser | How it is launched | Profile selection |
|---|---|---|
| Dia, Arc | AppleScript | profile |
| Chrome, Chromium, Brave, Edge, Vivaldi, Comet | `open` with `--profile-directory` | profile |
| Firefox, Zen | `open` with `-P` | profile |
| Safari, Orion | `open` | - |

The table grows as browsers are added; a browser that is not listed gets the plain `open -a` fallback.

## Rules

Rules are a local file, edited on the Mac. The repository holds the format and examples with
placeholder hosts only. See `examples/` once the format lands.

## Known limit

Links clicked **inside** a browser stay in that browser. Only links coming from other apps are routed.

## Status

Early development; not released yet.
