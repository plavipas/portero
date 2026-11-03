# ADR 0007: A failed "Remember" is shown as a native notification

Status: accepted by the user, 2026-11-03 (DEV-50); chosen in the brief (a native macOS notification, permission
asked once, short reason plus a hint to the log; the link still opens where it was sent).

Extends ADR 0005 decision 5 and ADR 0006 decision 4 ("a failed write never blocks the link: it opens first, the
failure is logged"). Everything else stands: one local YAML file, the link opens first, the CLI never asks and
never writes, nothing in the repository is personal.

## Context

When "Remember" cannot write the rules file (offloaded iCloud file, lock held, symlink, parse or verification
error, disk full) the only trace is a `remember failed: <reason>` part in `decisions.log`. The user ticks the box,
the link opens, and the next link to the same host asks again with no hint why.

## Decisions

1. **Notification.** After the link has opened, a failed "Remember" (the ADR 0005 panel and the ADR 0006 panel
   alike) posts one macOS notification through `UNUserNotificationCenter`: title `Portero`, body
   `Could not remember <host>: <short reason>. Details: ~/.config/portero/decisions.log` (one line, control
   characters stripped, cut to a sensible length). One fixed request identifier, so repeated failures replace
   each other instead of piling up; no sound. A successful "Remember" never notifies.
2. **Permission once.** The first failure asks macOS for permission (`UNAuthorizationOptionAlert`); the answer is
   kept by macOS. Denied or not determined: nothing is shown, the log says so. While the permission prompt or the
   delivery is pending the link counts as in flight, so the app does not quit under the prompt; the wait is
   capped (60 s for the prompt, 5 s for delivery) and the link is already open, so nothing is ever delayed.
3. **Only inside `Portero.app`.** Notifications need a bundled, signed process (the ad-hoc signature of
   `scripts/build-app.sh` is enough); a bare binary or the CLI gets none and must not try (the framework raises
   when there is no bundle). The CLI never remembers, so it never notifies. The `UserNotifications` framework is
   linked in `internal/handler` only, behind the darwin build tag; other platforms log only.
4. **Shown even when Portero is active.** The app is the notification center's delegate and presents banners in
   the foreground; otherwise macOS would drop the banner when the panel had made Portero the active app.
5. **Log.** The `remember failed: <reason>` part of the sixth column (ADR 0005 decision 7, ADR 0006 decision 6)
   is written to the log first, then the notification is posted: the wait for the permission prompt can last up
   to 65 s and a quit during it must not lose the line the notification points to. The outcome of the notification
   (`notified`, `notification denied`, `notification: no answer`, `notification failed: <why>`) goes to the
   handler's stderr, not to the log line. The third column of a link settled by the unmatched panel says `none` instead
   of `default` (the link had no rule; the sixth column says what was chosen); the diagram draws these lines
   through a "no rule" node.
6. **Clarified ADR 0006 decision 4.** A remembered host rule carries no opener condition: it applies to links
   from every app. The checkbox says so: `Remember for this host (<host>), links from any app`. A host with
   non-ASCII labels shows both forms, `<unicode> = <punycode>`; an `xn--` host shows `<punycode> = <unicode>`,
   so a homograph cannot hide behind one of them. The rule written is unchanged (the lower-case host as the link
   carries it).

## Consequences

- Portero appears under System Settings > Notifications after the first failed "Remember"; a user who denies gets
  log lines only.
- A copy of the app without a signature (the ad-hoc step failed at build time) shows nothing and logs
  `notification failed`.
- The log gains a new third-column value, `none`; readers of old lines are unaffected.
