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
- Logs every routing decision. A missing or broken rules file never swallows a link: it opens in Safari and the
  error is logged.
- Ships a CLI: `portero open <url>`, `portero rules`, `portero sync`, usable as `$BROWSER`.

## Browser support

| Browser | How it is launched | Profile selection |
|---|---|---|
| Dia | AppleScript: new tab in the named profile of window 1; launches the app first when no window is open | profile name |
| Arc | AppleScript: new tab in the named space (written from the dictionary, not yet verified live) | space title |
| Chrome, Chromium, Brave, Edge, Vivaldi, Opera, Comet | `open -b <bundle> -n --args --profile-directory=<dir>`; the directory is looked up by the profile's display name in Local State | profile name or directory |
| Firefox, Firefox Developer Edition, Zen | `open -a <app> -n --args -P <name>`; names from profiles.ini | profile name |
| Safari, Orion | `open -a` | - |

A browser that is not in the table gets a plain `open -a <name>` (or `open -b` for a bundle id).
`portero browsers` lists the installed ones with the profile names it found.

## Rules

Rules are a local file, edited on the Mac. The repository holds the format and examples with
placeholder hosts only. See `examples/` once the format lands.

## Known limit

Links clicked **inside** a browser stay in that browser. Only links coming from other apps are routed.
A companion browser extension that hands such clicks to Portero is on the list for a later version.

## Install (from source, macOS)

```sh
make install              # builds build/Portero.app, copies it to ~/Applications, links ~/.local/bin/portero
portero rules init        # writes the example rules to ~/.config/portero/rules.yaml; edit it
portero browsers          # the browsers and profile names Portero sees on this Mac
portero open https://example.com/ --dry-run
portero default set       # macOS asks to confirm Portero as the default browser
portero sync icloud       # optional: keep rules.yaml in iCloud Drive, shared between your Macs
```

`make build` gives a bare CLI in `bin/portero` (no link handling), `make test` runs vet and tests.

## Status

Early development; not released yet.
