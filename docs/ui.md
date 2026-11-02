# Local web UI (`portero serve`)

`portero serve` starts a small HTTP server on a loopback address and serves one page: a live diagram of where
links go and an editor for the rules file. Nothing leaves the Mac. The design decisions are in
[ADR 0002](adr/0002-local-web-ui.md).

## Start

```sh
portero serve                          # random free port on 127.0.0.1, opens the page
portero serve --port 8765              # fixed port on 127.0.0.1
portero serve --listen 127.0.0.1:8765  # same, as an address; localhost is accepted and becomes 127.0.0.1
portero serve --no-open                # only print the URL
```

The server prints `Portero UI: http://127.0.0.1:<port>/auth/<nonce>  (Ctrl-C stops it)`; the link works once (see Write protection). Ctrl-C (or `SIGTERM`) stops it.

- Only loopback addresses are accepted. `--listen 0.0.0.0:8765` or any other non-loopback address is refused
  with an error. `--port` and `--listen` cannot be used together.
- Without `--no-open` the page is opened the way `portero open` would open it: through your own rules, with the
  shell's working directory, so a `cwd` rule or the default picks the browser. The decision is logged. When the
  rules file cannot be read, the page opens in Safari and an `ERROR` line is logged.

## The page

Left side:

- The routing diagram: openers (the apps that sent links) -> rules -> targets (browser, profile, container).
  It shows the rules of the file and the last 200 decisions of the log. The width of an edge grows with the number
  of logged decisions that took it; an edge with no decisions is drawn thin. The path of the last decision is
  animated. Hovering a node highlights its edges. A rule that the log mentions but the file no longer has is
  marked as stale; a target whose browser is not installed is marked too.
- The last decisions (up to 20, newest first): time, opener, rule, target, URL. An `ERROR` line is highlighted.
  `show` animates that decision's path in the diagram.
- The browsers Portero sees on this Mac, with their profiles.
- A theme toggle (dark / light) in the header; the choice is kept in the browser's local storage. Without a stored
  choice the page follows the system setting.

The page asks the server for new log lines every 2 seconds and redraws the diagram when there are any.

Right side, the editor of the rules file:

- **Validate** runs the checks of `portero rules check` on the text in the editor. It also runs by itself shortly
  after you stop typing. An error is shown with its line number, and the line is marked in the gutter. Warnings
  (a browser that is not installed, a missing container, and so on) are listed but do not block Apply. The
  **Changes** panel shows a line diff against the file on disk.
- **Stale detection**: the page remembers the SHA-256 of the file it loaded. If the file on disk changed since
  then, Validate says so and Apply is refused; **Reload** loads the current file.
- **Match a link**: URL, opener (bundle id, optional) and directory (optional) are matched against the text in
  the editor, not against the saved file. The result shows the rule, the target and, on macOS, the command that
  would run (as `--dry-run` prints it); the path is animated in the diagram. Without a directory the `cwd` rules
  are skipped and the result says so.
- **Apply** is enabled only when the text differs from the file, the last validation passed and the file on disk
  is unchanged. The server validates the text again, then writes exactly the bytes from the editor through a
  temporary file and a rename, mode 0600. The previous file is kept next to it as `rules.yaml.bak` (mode 0600).
  A rules path that is a symlink is refused; edit the target file by hand.

## API

The page is the only intended client. All responses are JSON. No path is ever taken from a request: the server
works on the rules file that `portero rules path` resolves and on `decisions.log` in the config directory.

| Method and path | Body | Returns |
|---|---|---|
| `GET /api/state` | - | rules path and place (`local` / `icloud`), file text and SHA-256, parse error with line, warnings, rules summary, browsers, the last 200 decisions, log offset, graph |
| `POST /api/validate` | `{"yaml", "base_hash"}` | `ok`, `error` (message, line), `warnings`, `summary`, `diff`, `current_hash`, `stale` |
| `POST /api/match` | `{"yaml", "url", "opener", "cwd"}` | `rule`, `target`, `cwd_used` / `cwd_note`, `path`, `plan` (macOS); an empty `yaml` matches against the saved file |
| `POST /api/apply` | `{"yaml", "base_hash"}` | `hash`, `warnings`, `backup`; 422 on a parse error or a symlinked rules path, 409 when the file on disk changed |
| `GET /api/decisions` | - | without `offset`: the tail of the log; with `offset=N`: the decisions that follow byte offset N. `limit` (default 200, at most 1000). Returns `decisions`, `offset`, `truncated`, `skipped` |
| `GET /api/graph` | - | the diagram: openers, rules, targets, edges with counts |

## Write protection

Loopback is reachable by every local process, by other macOS users on this Mac and by containers and VMs, so
checking `Host` and `Origin` is not enough. The server therefore has a single-use link and these checks
(`internal/serve/guard.go`):

- **A single-use link.** On every start the server makes a one-time nonce (32 random bytes, hex) and prints
  `http://127.0.0.1:<port>/auth/<nonce>`; the browser that `portero serve` opens gets the same link. The first
  `GET /auth/<nonce>` (constant-time compare) sets the session cookie `portero_ui_<port>` (`HttpOnly`,
  `SameSite=Strict`, `Path=/`) and redirects to `/`. The cookie carries a different random secret, kept per user in the cache directory (`~/Library/Caches/portero/ui-session` on macOS; mode 0600, created on the first run, rotated after seven days, never under the config directory so dotfiles repositories and folder sync do not carry it; delete the file to reset every session, see ADR 0003) so that a page opened earlier survives a server restart on the same `--port` with a plain reload; the secret is never in
  a URL, a log line or a process's argv. The link works once: a second use, or a wrong nonce, gets 403, and a
  second use of the right nonce prints `portero serve: the UI link was used again; if that was not you, stop the
  server` once, so a link taken from `ps` or the browser history shows up. After use the link in the browser
  history is a dead nonce.
- Every `/api/*` request and every page file needs the session cookie, compared in constant time; without it the
  answer is 403 ("open the URL printed by portero serve"). The decision log records the bare address, never the
  nonce.
- The port in the cookie name only keeps two Portero servers on `127.0.0.1` from overwriting each other's cookie.
  Browsers store cookies per host, not per port, and send a `127.0.0.1` cookie to every port, so a local page the
  user opens on another port can receive it. The risk is low; a header-based design is a possible follow-up.
- It listens on a loopback address only.
- `Host` must be the bound address, or `localhost` / `127.0.0.1` with the same port. Anything else gets 403, which
  stops DNS rebinding.
- Every `POST` needs an `Origin` header exactly equal to `http://` + `Host` (a missing or `null` origin gets 403)
  and `Content-Type: application/json` (parameters such as `charset` are allowed; anything else gets 415).
- Request bodies are capped at 1 MiB.
- No CORS headers are sent, so other origins cannot read the responses.
- Every response has `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`. The page is served with a
  Content Security Policy that allows only the server's own scripts and styles (no inline script or style, no
  external loads) and `Referrer-Policy: no-referrer`. Log content is rendered as text only.
- Only `GET`, `HEAD` and `POST` are accepted.

As a result, a site open in any browser can neither read nor change the rules through the server.

The page is self-contained: plain HTML, CSS and JS embedded in the binary with `go:embed`; no node toolchain, no
CDN, no fonts.

## Limits

- The rules path is resolved on every request, so `portero sync icloud` or `portero sync off` while the server runs
  is picked up. A page loaded before the switch sees the other file as changed and refuses Apply until Reload.
- With the rules file in iCloud Drive, `rules.yaml.bak` is written to iCloud Drive too.
- Only one backup is kept; each Apply overwrites `rules.yaml.bak`.
- The server reads the decision log and never writes it.
- A settings page and a native window are planned.
