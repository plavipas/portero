# ADR 0002: Local web UI (`portero serve`)

Status: accepted, 2026-11-02 (DEV-24)

Refines ADR 0001, decision 2 ("edited by hand in v1"): the rules file can also be edited through a local web
page. Everything else in ADR 0001 stands: one local YAML file, no service, no account, portable core.

## Decisions

1. **`portero serve` is a loopback-only HTTP server** (`internal/serve`, standard library `net/http`,
   `go:embed`). It binds `127.0.0.1` on a random free port by default (`--port N`, `--listen 127.0.0.1:N`);
   a non-loopback listen address is refused. Loopback is not private on this Mac: every local process, every other
   macOS user (Fast User Switching) and containers or VMs reach `127.0.0.1`. That is why decision 3 adds a
   per-run token.
2. **The page is self-contained**: plain HTML, CSS and JS embedded in the binary; no node toolchain, no external
   loads (no CDN, no fonts, no analytics). The diagram is drawn by the page's own JS as SVG, in the look of the
   archify diagrams (shared theme tokens), not by the archify tool at runtime.
3. **Every request is guarded**. `portero serve` prints (and opens) a single-use link `/auth/<nonce>` (nonce: 32
   random bytes from `crypto/rand`, hex). The first `GET /auth/<nonce>` sets an `HttpOnly; SameSite=Strict`
   cookie that carries a separate random session secret, never put in a URL, and redirects to `/`; a second use
   or a wrong nonce is 403, and a second use of the right nonce is reported on stdout, so a link read from the
   argv of the browser start (`ps` shows other users' arguments) cannot be replayed unnoticed. Every API call
   and every UI file needs the cookie (constant-time compare), so a local process, another macOS user or a
   container that merely reaches the port can neither read the log nor write the rules. The nonce is never
   written to the decision log. The port in the cookie name only keeps two servers apart: browsers send a
   `127.0.0.1` cookie to every port (low risk; a header-based design is a possible follow-up). On top of that,
   `Host` must be the bound address (or `localhost:<port>`), `Origin` must be
   present and equal to the page's own origin, `Content-Type` must be `application/json`, the body is capped.
   A cross-site page in any browser can neither read nor change the rules through the server.
4. **The server writes exactly one file**: the rules file that `config.RulesFile()` resolves. The path never
   comes from the client. A write happens only on an explicit Apply, only when `rules.Parse` accepts the text
   (warnings are allowed), only when the file on disk still has the hash the page loaded (otherwise 409), and
   the previous content is kept as `rules.yaml.bak` next to the file. The write is atomic (temp file + rename),
   mode 0600.
5. **The server reads the decision log, never writes it**; the UI shows the tail of the log and polls for new
   lines. Log content (URLs, bundle ids) is untrusted and is rendered as text only (strict CSP, no inline
   scripts, no `innerHTML` with data).
6. **The link handler is untouched**: `portero serve` does not change how a link is routed; it only shows and
   edits the file the handler reads.

## Consequences

- `internal/serve` stays portable (no cgo, no `os/exec`); opening the page in a browser at start is done by
  `cmd/portero` through the existing `launch` package, routed by the user's own rules like `portero open`.
- A Settings page and a native window are separate work (DEV-39).
- On a Mac with the rules file in iCloud Drive, `rules.yaml.bak` lands in iCloud Drive too.
