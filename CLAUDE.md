# Portero - working rules

Read README.md first. Everything in the repository in English: code, comments, docs, commits, PR texts.

- Portero is a general tool: no personal hostnames, workspace names, profile names or
  bundle ids of private tools anywhere in the repository; only placeholders.
- Single-instance browsers (Dia, Arc) are driven through AppleScript
  (`/Applications/Dia.app/Contents/Resources/Dia.sdef`): profiles are addressed through a window;
  with no window open, fall back to `open -a <Browser> <url>` and move the tab.
- Never change the macOS default browser or `$BROWSER` from tests.
- Go, standard library plus gopkg.in/yaml.v3; `make test` (= `go vet ./... && go test ./...`) green before a commit.
- macOS-specific parts (URL handler app, opener detection, AppleScript) stay behind build tags; the core
  (rules, matching, config, log) is portable.
