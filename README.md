<p align="left"><img src="docs/logo.svg" width="64" height="64" alt="Portero logo"></p>

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
- Opens local HTML files too (Open With, drag onto the icon): they go to the default browser and profile.
- Logs every routing decision. A missing or broken rules file never swallows a link: it opens in Safari and the
  error is logged.
- Ships a CLI: `portero open <url>`, `portero rules`, `portero sync`, usable as `$BROWSER`.

## How it works

macOS hands every `http`/`https` link from another app to Portero.app as an Apple event, together with the
sender's process, from which Portero reads the opener's bundle id. The first rule that matches the host, path and
opener picks the browser and profile; a launcher opens the link there and the decision goes to
`~/.config/portero/decisions.log`. When the rules file cannot be read, the link opens in Safari and an `ERROR`
line is logged.

![How Portero works](docs/diagrams/how-it-works.png)

[Interactive version](docs/diagrams/how-it-works.html)

## Browser support

| Browser | How it is launched | Profile selection |
|---|---|---|
| Dia | AppleScript: new tab in the named profile of window 1; launches the app first when no window is open | profile name |
| Arc | AppleScript: new tab in the named space (written from the dictionary, not yet verified live) | space title |
| Chrome, Chromium, Brave, Edge, Vivaldi, Opera, Comet | `open -b <bundle> -n --args --profile-directory=<dir>`; the directory is looked up by the profile's display name in Local State | profile name or directory |
| Firefox, Firefox Developer Edition, Zen | `open -a <app> -n --args -P <name>`; names from profiles.ini. With a container: the link is passed as `ext+container:name=<container>&url=<url>` (only when the "Open external links in a container" extension is confirmed active in the profile; otherwise the plain link) | profile name, container name |
| Safari, Orion | `open -a` | - |

A browser that is not in the table gets a plain `open -a <name>` (or `open -b` for a bundle id).
`portero browsers` lists the installed ones with the profile names it found and, for the Firefox family, the
containers of each profile and whether the container extension is active.

## Rules

Rules are a local file, edited on the Mac. The repository holds the format and examples with
placeholder hosts only: [docs/rules.md](docs/rules.md) and [examples/rules.yaml](examples/rules.yaml).

```yaml
- name: team wiki
  match: { host: wiki.example.com }
  open: { browser: Zen, container: Work }   # Firefox family only; in Zen the container picks the Space
```

`portero rules check` validates the file and warns about browsers, containers or the container extension that
are missing on this Mac.

## Known limit

Links clicked **inside** a browser stay in that browser. Only links coming from other apps are routed.
A companion browser extension that hands such clicks to Portero is on the list for a later version.

A container link (`ext+container:`) sent to a Firefox-family browser that is still starting, for example while it
applies an update, can be dropped before the extension is ready. Opening the link again works.

## Install (from source, macOS)

```sh
make install              # rebuilds build/Portero.app, copies it to ~/Applications, links ~/.local/bin/portero
portero rules init        # writes the example rules to ~/.config/portero/rules.yaml; edit it
portero browsers          # the browsers and profile names Portero sees on this Mac
portero open https://example.com/ --dry-run
portero default set       # macOS asks to confirm Portero as the default browser
portero sync icloud       # optional: keep rules.yaml in iCloud Drive, shared between your Macs
```

`portero default set` is safe to run again: it prints `already set` for each scheme Portero already handles and
exits 0. A scheme that macOS still does not hand over to Portero is reported as an error.

The app icon comes from `packaging/Portero.icns`; `make icon` (`scripts/make-icon.sh`) regenerates it from
`docs/logo.svg` when the logo changes.

`make build` gives a bare CLI in `bin/portero` (no link handling), `make test` runs vet and tests.

## Roadmap

- A companion browser extension, so links clicked inside a browser are routed too.
- A local UI: an interactive diagram of where links go, upload and edit of the rules file, settings.
- More sync backends for the rules file (a private git repository, Google Drive).
- Other operating systems.

## Status

Early development; not released yet.
