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
- Fixed decisions live in docs/adr/0001-foundations.md; changing one needs a new ADR and the user's "да".

## Agents (`.claude/agents/`)
- `brief` (sonnet) - explains the task, asks ONE question per reply with a recommendation until "Вопросов нет". Read-only.
- `architect` (fable) - the planning pass: inspects the repository and the installed app, returns the plan with
  acceptance criteria; designs and ADRs. Read-only.
- `go-developer` (sonnet) - rules, browsers, launchers, CLI, sync, the cgo/Objective-C handler; unit tests; local commits only.
- `qa-engineer` (opus) - adversarial verification of the acceptance criteria: unit tests, `--dry-run`, the installed
  app on this Mac (closes every tab it opens, never sets the default browser). PASS/FAIL.
- `security-reviewer` (opus) - read-only review of the branch diff: argv/AppleScript injection, lost or misrouted links,
  files and permissions, install scripts, leaked personal data. APPROVE / CHANGES NEEDED.
- `tech-writer` (opus) - README, docs/, ADRs, archify diagrams; runs after qa PASS when documented behaviour changed
  (step 5a). Docs-only commits, never pushes.

## Task flow
Tasks are Linear issues of the personal workspace (project Portero); the key is `DEV-<n>` / `DEVOPS-<n>` (teams
Dev / DevOps). The main session puts the issue text into the agent prompt. One task - one branch - one session.
Bigger work without an issue ("обсудим", "брейншторм", "оцени", a new feature) starts with `brief` on the user's
words under a temporary key `ADHOC-<slug>`; the Linear issue is created from the brief's ЗАДАЧА ДЛЯ ТРЕКЕРА text
only after the user's "да", then the DEV key takes over. Small fixes (one file, one obvious change, review findings
on an existing PR) are done directly: no issue, no agents, no markers.

The order is enforced by `.claude/hooks/flow-guard.py` (PreToolUse on Agent/Skill): architect, go-developer,
qa-engineer, cross-review and security-reviewer are blocked for a key until the previous step's marker exists in
`.claude/flow-state/<KEY>.<step>`. The developer, qa (PASS) and security (APPROVE) write their own markers; the main
session writes `brief` after "Вопросов нет", `plan` after the user's "да" on the plan and `crossreview` after showing
the cross-review summary, via `.claude/hooks/flow-mark`. Marking a step again clears every later marker. A step is
skipped only on the user's explicit words (`--skipped-by-user "<words>"`).

1. **Current state first.** Before any agent: branch, open PRs, latest release, `portero default status`.
2. **brief** with the key: one question per reply, discussed to the end; "ок" / "дальше" / "как рекомендуешь"
   accepts the recommendation.
3. **architect** with the key, the brief report and all answers -> plan with acceptance criteria and risks. Together
   with the plan ask which AI does the cross-review (Codex default / DeepSeek), once per task. Nothing changes
   before the user's "да"/"начинать".
4. **go-developer** -> feature branch, implementation, tests, local commit. Never pushes.
5. **qa-engineer** -> adversarial verification; pass it only the task goal, the acceptance criteria and the diff.
   PASS required; FAIL goes back to the developer automatically.
5a. **tech-writer** -> right after qa PASS, without a question, when the change touches documented behaviour (CLI,
   rules format, browser table, handler, sync, install, agents, task flow). Docs-only commit; does not restart qa.
6. **cross-review** skill with the AI chosen in step 3, in a separate Orca tab; findings back to step 4.
7. **security-reviewer** on the branch diff; APPROVE required, otherwise back to step 4.
8. **push / PR** (no approval needed), then the `gitreason` workflow comments the notes of the PR and the Claude Code
   Review workflow reviews it; read it back with the `claude-pr-review` skill. Real findings go back to step 4.
9. **Merge** only on the user's "да" (merge commit, never squash, so the per-commit notes survive). Then a work
   summary (Сделано / Найдено / Осталось).

**Review loop - automatic, no questions** from the first developer commit to the PR: (a) developer <-> qa until
PASS; (b) cross-review findings -> (a); (c) security findings -> (a), APPROVE -> (d); (d) push / PR, wait for the
Claude review; (e) Claude review findings -> (a). A finding is fixed unless verified as a false positive; a false
positive is reported with the evidence. The loop never replies on GitHub.

## Local testing on this Mac
`make install` refreshes `~/Applications/Portero.app` and `~/.local/bin/portero`. A link can be sent to the app
without making it the default browser: `open -a Portero <url>`. Agents close every tab they open and never run
`portero default set`; the user does that once and confirms the macOS dialog.
