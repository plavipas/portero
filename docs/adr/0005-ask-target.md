# ADR 0005: Ask target - a panel for links no rule can place, and "Remember" writes the rules file

Status: accepted, 2026-11-03 (DEV-47); accepted by the user on 2026-11-03.

Refines ADR 0001 decision 2 ("rules are edited by hand") and decision 5 (the handler app quits two seconds after
the last link), and ADR 0002 decision 4 ("the server writes exactly one file"): the link handler becomes a second
writer of the rules file. Everything else stands: one local YAML file, `version: 1`, no service, a link is never
lost.

## Context

Some links carry nothing a rule can use to pick a profile: a meeting link from Calendar (one opener for every
account), Google Meet (one host for every account), Zoom (a shared host such as `us06web.zoom.us`). They fall to
the default target, often the wrong profile.

## Decisions

1. **`ask` in `open`.** A plain rule's or a group's `open` may carry `ask:` with a list of 1 to 9 targets instead
   of `browser`/`profile`/`container`; the two forms are exclusive. The file stays `version: 1`; `default` cannot
   ask. A rule inside a group never has `open`, as before.
2. **The handler shows a panel.** For a link whose decision asks, `Portero.app` shows a small floating panel near
   the mouse pointer (an `NSPanel` that does not activate the app when possible), one button per target labelled
   `Browser · Profile`, keys 1 to 9, and a "Remember" checkbox. Esc or closing the panel opens the link in the rules
   file's `default` target. There is no timeout. Each pending link has its own panel; other links are routed as
   before while panels are open. An identical link (same URL and opener) arriving while its panel is open joins
   that panel and is opened once.
3. **The app stays alive while a panel is open**: a pending ask counts as a link in flight. When macOS asks the
   app to quit (logout, restart) with panels open, every pending link is opened in `default` and logged as
   cancelled before the app quits.
4. **"Remember" adds a plain rule**: host plus the meeting path (`/abc-defg-hij` for Google Meet, `/j/<id>` for
   Zoom, the whole path otherwise; the query is ignored) to the chosen target, inserted as a top-level element
   directly before the asking rule or, when a group asks, before the group (children of a group cannot carry a
   target of their own). The insertion is textual, so comments and formatting of the rest of the file are kept.
   No "Remember" when the link has no path to pin (`/` or empty).
5. **One write path.** The handler and `portero serve` write through the same code: an exclusive lock on
   `rules.lock` in the config directory, symlink refused, offloaded iCloud file refused, the new text must parse
   and must route the remembered link to the chosen target with every other rule unchanged, the previous content
   kept as `.bak`, temp file + fsync + rename, mode 0600, and the file re-checked against the hash it was built
   from just before the rename. A failed write never blocks the link: it opens first, the failure is logged.
6. **The CLI never shows a panel or prompts**: `portero open` and the page `portero serve` opens use the
   `default` target for an asking rule and say so; `--dry-run` and `rules match` print the ask list.
7. **The log** keeps its five columns and may add a sixth: `asked -> <target>`, `asked -> <target>, remembered
   "<name>"`, `asked -> default (cancelled)`, `asked -> default (CLI)`.

## Consequences

- A rules file installed from a template drifts once "Remember" is used; the tool that installs it reports that.
- A remembered rule names a browser of this Mac; on another Mac sharing the file through iCloud Drive
  `rules check` warns when that browser is missing.
- `rules.yaml.bak` is overwritten by whichever writer saved last (UI or "Remember").
- The handler now draws UI on the main thread; the Go side must keep the app loop on the main OS thread.
