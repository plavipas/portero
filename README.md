# Portero

Portero (Spanish for *doorman*) is a small macOS default-browser router. It registers itself as the
handler for `http`/`https` and sends every link opened from another app (Slack, Telegram, Mail,
terminal, hooks, `$BROWSER`) to the right browser and profile:

- hosts of one work context -> Zen (its own Space),
- client hosts/paths -> the matching Dia profile (Dia profiles are driven over AppleScript, because
  Dia runs a single instance and ignores `--profile-directory` arguments),
- everything else -> a default Dia profile.

## Why not finicky

finicky launches browsers with `open -n -a <app> --args ...`; Dia (like Arc) refuses a second instance
and ignores the arguments while running, so a Dia profile cannot be chosen that way
(finicky issues #456, #502). Portero talks to Dia's AppleScript dictionary instead.

## Scope of v1

- macOS app registered for `http`/`https` + CLI `portero open|rules|sync`.
- Rules live in a local file on each Mac, stored under iCloud Drive so Apple syncs it between the
  user's Macs. Matching by host wildcard, path prefix and the opener app's bundle id.
- A log of routing decisions.
- Installed and set as the default browser by the machine setup (`harness apply`), which also points
  `$BROWSER` at `portero open`.

Later: pluggable sync backends (private GitHub repo, Google Drive, ...), then a GUI for the rules.

## Known limit

Links clicked **inside** a browser stay in that browser. Only links coming from other apps are routed.

## Rules

Real hostnames of clients belong in the local rules file, never in this repository. See
`examples/` for the format with placeholder hosts.
