# ADR 0008: The `portero://` URL scheme, an entry point for web pages

Status: accepted, 2026-11-15 (DEV-95); the entry point and the routing were chosen by the user in the brief.

Extends ADR 0001 decision 5 (the handler app) with a second kind of link the app receives. Everything else
stands: one local YAML file, a missing or broken rules file opens the link in Safari, nothing personal in the
repository. The companion browser extension stays a later step (DEV-21).

## Context

Links clicked inside a browser never reach Portero, so no rule fires for them: a "Join" button on a local
dashboard page opens the meeting in the profile the page happens to live in, and an `ask` rule for meetings is
never consulted. A page needs a link form that hands the click to Portero. A loopback HTTP endpoint and a browser
extension were considered in the brief and not chosen for this step.

## Decisions

1. **Scheme.** `Portero.app` registers the URL scheme `portero` (a second entry in `CFBundleURLTypes`). The only
   form is `portero://open?url=<percent-encoded http(s) URL>`. macOS delivers it through the same Apple event as
   `http`/`https` links, so the opener is the browser the click came from.
2. **Routing.** The inner URL is routed exactly like the same link coming from any app: all rules, `ask` panels
   including `ask_unmatched`, "Remember", redirect unwrapping, the Safari fallback for a missing, broken or
   offloaded rules file. The browser is given the inner URL, never the `portero://` link.
3. **Strict parsing, no recursion.** Accepted: host `open` (any letter case), an empty path or `/`, no user
   info, no port, no fragment, and a query with exactly one parameter, `url`, given once. The value is decoded
   once and must be an `http` or `https` URL with a host, at most 8 KiB. Everything else is rejected: another
   action, a missing, empty or repeated `url`, any other parameter, a fragment outside the encoded value, `;` in
   the query, an inner `portero://`, `file://` or any other scheme, a double-encoded value. Unknown parameters
   and fragments are rejected rather than ignored because they are what an unencoded inner URL
   (`?url=https://a/b?x=1&y=2#f`) turns into: a silently truncated link is worse than a refused one. The inner URL
   is never unwrapped as a `portero://` link again, so a link cannot loop.
4. **A rejected link opens nothing** and writes an `ERROR` line to `decisions.log` with the opener, the reason
   (`portero link rejected: <fixed reason>`) and the quoted link. This is the one case where the handler does
   not open a link: there is no safe URL to open.
5. **Log.** An accepted link writes the usual line with the inner URL in the url column and `via portero://` in
   the note column, after an ask note and before `unwrapped <url>`, joined with `; `.
6. **CLI.** `portero open` and `portero rules match` accept the same form and print `portero link: <inner URL>`
   before the decision, so the scheme can be checked with `--dry-run` and a `portero://` link is never handed to a
   browser (which would hand it back). A rejected link is an error on stderr, exit 1.
7. **Not a browser, not a default.** `portero` is not part of `handler.Schemes`: `portero default status|set`,
   the Settings page and the browser table do not know it. No rule can target it.

## Consequences

- The trust boundary moves: any web page can ask the browser to hand a link to Portero. The browser's own
  external-protocol prompt is the gate (the handler cannot see the page's origin), and what a page gains is what
  every app on the Mac already has: opening an `http(s)` URL in the browser and profile the rules pick, or
  showing the ask panel. Chromium offers "Always allow" only to potentially trustworthy origins (`https`,
  `http://localhost`, `http://127.0.0.1`) and stores it per origin and scheme.
- A click on a `portero://` link does nothing visible when Portero is not installed or the link is rejected; page
  authors are told so in the README.
- Future actions (`portero://<other>`) need a new ADR; older builds reject them.
