# ADR 0004: The UI can set the default browser and switch sync

Status: accepted, 2026-11-02 (DEV-39).

Refines ADR 0002, decision 4 ("the server writes exactly one file"). The rules editor keeps that rule. The new
Settings section of `portero serve` adds two more writes, each only on an explicit, confirmed click.

## Decision

1. **Default browser**: `POST /api/settings/default` runs the same code as `portero default set` (`ensureDefault`
   in `cmd/portero`). macOS shows its own confirmation dialog; the server never bypasses it, and the result is
   the handler read back from Launch Services, not the return value of the call. Only on macOS and only when the
   server runs from `Portero.app` (directly or through the `~/.local/bin/portero` symlink); elsewhere the
   endpoint answers 501 and the page hides the button. One request at a time; a second one while macOS waits for
   the dialog gets 409.
2. **Sync**: `POST /api/settings/sync` with `{"mode": "icloud" | "off"}` runs the same code as
   `portero sync icloud|off` (`internal/sync`). That moves the rules file into iCloud Drive (the local file is kept
   as `rules.yaml.local-backup`) or copies it back, and writes or removes `config.yaml` in the config directory.
   It holds the same lock as Apply, so an Apply and a switch never interleave. When `PORTERO_RULES` pins the rules
   path the switch is refused (409): the config file would not change the path in use.
3. **No path, bundle id or app path comes from a request**: the body carries only the mode; the app path is
   `appBundle()` of the running binary.
4. **The page confirms first**: both buttons open a confirmation that says what will change (the macOS dialog will
   appear; the rules file will be moved or copied, with both paths). The server does not rely on it: the guards of
   ADR 0002 decision 3 (session cookie, Host, Origin, Content-Type) apply to both endpoints unchanged.
5. **`internal/serve` stays portable**: it gets these actions as functions in `Options`; tests pass fakes, so no
   test changes the real default browser or the user's sync.

## Consequences

- A stolen session cookie can now also move the rules file between local and iCloud Drive and start the macOS
  default-browser dialog (which still needs the user's click). It cannot pick any other app or path.
- A sync switch while the editor holds unsaved text makes the page's hash stale; the page reloads after the
  switch and warns about unsaved text in the confirmation.
