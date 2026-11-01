# Portero - working rules

Read README.md first. Everything in the repository in English: code, comments, docs, commits, PR texts.

- Portero is a general tool: no personal or client hostnames, workspace names, profile names or
  bundle ids of private tools anywhere in the repository; only placeholders.
- Single-instance browsers (Dia, Arc) are driven through AppleScript
  (`/Applications/Dia.app/Contents/Resources/Dia.sdef`): profiles are addressed through a window;
  with no window open, fall back to `open -a <Browser> <url>` and move the tab.
- Never change the macOS default browser or `$BROWSER` from tests.
- Build: `make build`, tests: `make test` (Swift Testing; with Command Line Tools only, the Makefile passes
  the macro plugin path). Keep `make test` green before a commit.
