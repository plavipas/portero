# ADR 0006: Ask for links no rule matches, and a target that follows `default`

Status: proposed, 2026-11-03 (DEV-49); needs the user's "да".

Extends ADR 0005 (the ask panel and "Remember"): the panel is no longer shown only for rules that carry `ask`,
and "Remember" gains a second insertion point and a new target form. ADR 0001 stands: one local YAML file,
`version: 1`, no service, a missing or broken rules file opens the link in Safari, a link is never lost.

## Context

A link from another app that no rule matches opens silently in `default`. The user only notices a wrong profile
after the page has loaded, and adding a rule for that host means editing the file by hand.

## Decisions

1. **Unmatched links from apps ask.** When the link handler (`Portero.app`) receives an `http`/`https` link and
   no rule matches it (after redirect unwrapping, ADR 0005 and DEV-48), it shows the ADR 0005 panel. Every link
   the handler receives counts as "from an app", with or without a detected opener. `portero open` (and
   `$BROWSER`), the page `portero serve` opens, `file://` links, an offloaded, missing or broken rules file never
   show it and behave as before.
2. **Choices.** First "Keep default" (the rules file's `default`), then every distinct target the file already
   uses, in file order: plain rules, groups, ask lists; targets equal to `default`, rules that follow `default`
   and browsers not installed on this Mac are left out; at most 9 buttons in total. With nothing to offer
   besides `default` the panel is not shown. Esc, closing the panel and the app quitting open `default`.
3. **A target that follows `default`:** `open: {default: true}` on a plain rule or a group. It is exclusive with
   `browser`/`profile`/`container` and `ask`; `default: false` is an error; `default` itself and ask items cannot
   use it. The link opens in whatever `default` is when it arrives, so changing `default` moves these hosts too.
4. **"Remember" for an unmatched link writes a host rule**: `match: {host: <host>}` (the host of the URL the rules
   decided on, lower case, no path, no opener) and either the chosen target or `{default: true}` for "Keep
   default". The rule is appended as the last top-level element of `rules`, so it can only catch links that no
   other element matches; the write path, lock and verification of ADR 0005 decision 5 apply (every existing
   element unchanged, the new element last, the link routes to it). If a rule matches the link by the time the
   file is written, nothing is written.
5. **Switch.** Top-level `ask_unmatched: false` turns it off; no key means on. The Settings page edits the key
   through the same line editor as `unwrap`, keeping comments and other keys byte for byte.
6. **Log.** The sixth column says `asked (no rule) -> <target>`, `asked (no rule) -> default (kept)`,
   `asked (no rule) -> default (cancelled)`, followed by `, remembered "<name>"` or `, remember failed: <reason>`.

## Consequences

- After the upgrade, every link from an app to a host without a rule waits in a panel until answered; panels have
  no timeout. `ask_unmatched: false` restores the old behaviour.
- Remembered host rules accumulate at the end of the file. A rule added by hand below them loses those hosts;
  `rules check` warns when a host-only rule shadows a later element for that host.
- A rules file installed from a template drifts with every "Remember" (as in ADR 0005).
