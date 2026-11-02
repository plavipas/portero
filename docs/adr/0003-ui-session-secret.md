# ADR 0003: The UI session secret outlives one `portero serve` run

Status: accepted, 2026-11-02 (fix-ui-node-details).

Refines ADR 0002, decision 3. The one-time link (`/auth/<nonce>`) stays per run. The secret behind the session
cookie no longer does.

## Decision

1. **One secret per user, kept on disk**: 32 random bytes (hex) in `ui-session` under the user's cache
   directory (`os.UserCacheDir()/portero/`, so `~/Library/Caches/portero/` on macOS), mode 0600 in a 0700
   directory. The config directory is not used because dotfiles repositories and folder sync tools often
   carry it; a cache directory is never expected to be synced, and losing the file only costs a new link.
2. **Reused only when safe**: the file must be a plain file (no symlink), readable by nobody else, 64 hex
   characters, and younger than seven days; anything else is replaced by a fresh secret. Two servers starting
   at once share the secret (an exclusive lock on `ui-session.lock` around the check and the replacement). Deleting the file resets every session.
3. **Why**: the user restarts the server often while the page is open and wants a plain reload to keep working
   (`--port N --no-open`), instead of a new tab for every restart. With a per-run secret every restart invalidated
   the cookie.
4. **What it costs**: a cookie stolen from the browser stays valid until the secret rotates (at most seven days)
   or the file is deleted, and any process running as the same user can read the file, which it could already do
   with the rules file and the decisions log. The single-use link, the Host/Origin/Content-Type guards and the
   loopback-only listener are unchanged.
