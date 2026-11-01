# Portero - working rules

Read README.md first. Everything in the repository in English: code, comments, docs, commits, PR texts.

- Client hostnames, workspace names and bundle ids of client tools never go into code, examples,
  tests or docs; only placeholders. The real rules file is local and lives outside the repo.
- Dia is driven through AppleScript (`/Applications/Dia.app/Contents/Resources/Dia.sdef`): profiles
  are addressed through a window; with no window open, fall back to `open -a Dia <url>` and move the tab.
- Never change the macOS default browser or `$BROWSER` from tests.
