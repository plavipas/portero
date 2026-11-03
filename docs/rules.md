# Rules file

One YAML file, local to the Mac. Portero looks for it in this order:

1. `$PORTERO_RULES`
2. `rules: <path>` in `~/.config/portero/config.yaml`
3. `~/.config/portero/rules.yaml`

`portero rules path` prints the file in use, `portero rules check` validates it, `portero rules match <url>
[--opener <bundle id>] [--cwd <dir>]` shows the decision without opening anything.
The file can also be edited in the local UI (`portero serve`, see [ui.md](ui.md)): it validates the text before
saving and keeps the previous file as `rules.yaml.bak`.

`portero rules check` also checks the targets against this Mac and prints a `warning:` line for:

- a browser that is not installed;
- a container that the profile does not have yet (the extension creates it on first use);
- a Firefox-family profile where the container extension is not active;
- a container rule whose extension state cannot be read (profile not found, `extensions.json` unreadable): such links open without a container;
- a rule none of whose `cwd` directories exist on this Mac (`none of its cwd directories exist on this Mac (...)`).
  One existing directory is enough, so a file shared between Macs can list the paths of each Mac. Inside a
  group the warning names the group and the rule: `group "Work", rule "CLI logins": ...`;
- a name used twice in the top-level list, by two plain rules, two groups or a group and a plain rule
  (`name "Work" is used twice; the diagram and the log merge them`);
- a rule name used twice inside one group (`group "Work": rule name "x" is used twice; the log merges them`).

The target of a group is checked once, as `group "Work": ...`, not once per rule in it.

Warnings do not fail the check (exit 0). A `container` on a browser outside the Firefox family is an error
(exit 1), and so is a `cwd` prefix that is not absolute and does not start with `~/`.

The last line counts the rules. Without groups it is `<path>: 3 rule(s), default Dia/Personal`. With groups the
rules inside them are counted one by one:

```text
/Users/me/.config/portero/rules.yaml: 7 rule(s): 2 plain, 5 in 1 group(s), default Dia/Personal
```

## Format

The file has `version: 1`, a `default` target and an ordered `rules` list. An element of the list is either a
plain rule or a group.

```yaml
version: 1
default: { browser: Dia, profile: Personal }
rules:
  # A plain rule: its own conditions and its own target.
  - name: optional label, shown in the log
    match:
      host: example.com | [a.com, b.com]   # exact; "*.a.com" = any subdomain and a.com; "*" elsewhere = glob
      path: /prefix | [/a, /b]             # segment prefix: /a matches /a and /a/..., not /ab
      opener: com.vendor.app | [...]       # bundle id of the app that opened the link
      cwd: ~/dir | [/abs/dir, ...]         # working directory of a shell command; see "Working directory"
    open: { browser: <app name or bundle id>, profile: <optional>, container: <optional, Firefox family> }
    # or: open: { ask: [ { browser: ..., profile: ... }, ... ] }   1 to 9 targets, see "Ask"

  # A group: one target for several conditions.
  - name: required for a group
    open: { browser: ..., profile: ..., container: ... }
    rules:
      - name: optional
        match: { ... }                     # the same keys as above; no open here
      - match: { ... }
```

- Rules are tried top to bottom; the first match wins; `default` applies when none does.
- Every key present under `match` must match; a rule needs at least one key.
- Host matching is case-insensitive; path matching is case-sensitive.
- `profile` means whatever the browser calls it: profile (Dia, Chrome, Firefox) or space (Arc). Name them the way
  you split your life, e.g. Personal and Work.
- `browser`, `profile` and `container` must not contain a tab or a line break and must not start or end with a
  space.

### Groups

A group collects several conditions that send links to the same place. Groups are optional; a file with plain
rules only is complete.

- A group has a `name`, one `open` target and a `rules` list. It has no `match` of its own.
- Each rule in the group has a `match` and an optional `name`, and no `open`. Any of them that matches sends the
  link to the group's target.
- A group stands at its position in the list. When the walk reaches it, its rules are tried top to bottom; the
  first one that matches decides. If none matches, the walk continues with the element after the group.
- Groups cannot contain groups.

### Names and labels

The log, `portero rules match`, `portero open --dry-run` and the UI name the rule that decided:

| Rule | Label |
|---|---|
| plain rule with a name | its name |
| plain rule without a name | `#n`, its position in the top-level list (groups count as one position each) |
| rule in a group, with a name | `Group / name` |
| rule in a group, without a name | the group name |
| no rule matched | `default` |

A name must not contain a tab or a line break, must not start or end with a space and must not contain ` / `
(space, slash, space), which separates the group from the rule. A slash without the spaces is fine.

### Errors

`portero rules check` fails (exit 1) and names the element when a group is malformed:

- `group #2 has no name`
- `group "Work" has an empty browser`
- `group "Work" has match; conditions go into its rules`
- `group "Work" has no rules`
- `group "Work", rule "GitHub organisation": open is set on the group only`
- `group "Work", rule "inner": groups cannot contain groups`
- `group "Work", rule #4 has no match conditions (host, path, opener or cwd)`

A rule inside a group without a name is `#n` in these messages, counted inside the group.

### Example

```yaml
version: 1
default: { browser: Dia, profile: Personal }
rules:
  - name: personal repositories          # before the group: wins for github.com/personal-account
    match: { host: github.com, path: /personal-account }
    open: { browser: Dia, profile: Personal }

  - name: Work
    open: { browser: Dia, profile: Work }
    rules:
      - name: hosts
        match: { host: ["*.example.com", tracker.example.net] }
      - name: GitHub organisation
        match: { host: github.com, path: /work-org }
      - name: Slack
        match: { opener: com.tinyspeck.slackmacgap }
      - name: CLI logins
        match: { cwd: ~/Documents/projects/work }
      - match: { host: chat.example.org }    # no name: logged as "Work"

  - match: { host: github.com }            # after the group: the rest of github.com
    open: { browser: Safari }
```

```text
$ portero rules match https://github.com/personal-account/x
Dia/Personal  (rule: personal repositories)
$ portero rules match https://github.com/work-org/x
Dia/Work  (rule: Work / GitHub organisation)
$ portero rules match https://github.com/other
Safari  (rule: #3)
$ portero rules match https://chat.example.org/
Dia/Work  (rule: Work)
$ portero rules match https://example.com/a --opener com.tinyspeck.slackmacgap
Dia/Work  (rule: Work / hosts)
```

The last link comes from Slack, but `hosts` stands above `Slack` in the group and matches first; the target is
the same either way. The plain rule without a name after the group is `#3`: the group takes position 2.
When the rules file has a `cwd` rule, as here, each answer is preceded by a `cwd: <dir>` line.

## Ask

Some links say nothing a rule can use: a meeting link from Calendar (one opener for every account), Google Meet
(one host for every account), Zoom (a shared host). `ask` lets the link handler offer a choice instead of a
fixed target.

```yaml
- name: Meetings
  match: { host: [meet.google.com, "*.zoom.us"] }
  open:
    ask:
      - { browser: Dia, profile: Personal }
      - { browser: Dia, profile: Work }
```

- `ask` replaces `browser`/`profile`/`container` in `open`; the two forms cannot be combined. It lists 1 to 9
  targets, each written like a normal target.
- It is allowed on a plain rule and on a group's `open`. It is not allowed on a rule inside a group (those have no
  `open`) or in `default`.
- **An ask on a group asks for every link that any child rule of the group matches.** The group's children cannot
  carry a target of their own, so there is no way to ask for only some of them; use a plain rule for that.
- Rules are still tried top to bottom. The first match decides, so a plain rule placed above an asking rule
  (for example one made by "Remember") wins.

When the link handler (`Portero.app`) routes a link to an asking rule, it shows a small panel near the mouse
pointer:

- one button per target, labelled `N  Browser · Profile` (with ` · Container` when set), and a "Remember"
  checkbox;
- keys 1 to 9 pick the target; Esc or closing the panel opens the link in the `default` target of the file;
- there is no timeout, and other links are routed as usual while a panel is open;
- the same link (same URL and opener) arriving while its panel is open joins that panel and opens once;
- up to 9 panels are shown at the same time, further ones wait in a queue;
- when macOS asks the app to quit with panels open, every pending link opens in `default` and is logged as
  cancelled.

### Remember

With "Remember" ticked, the chosen button also adds a plain rule for this meeting to the rules file:

```yaml
  # added by Portero (Remember) 2026-11-03
  - name: meet.google.com/abc-defg-hij
    match:
      host: meet.google.com
      path: /abc-defg-hij
    open:
      browser: Dia
      profile: Work
```

- The rule matches the host and the meeting path. Google Meet: the meeting code. Zoom: `/j/<id>`, `/w/<id>`,
  `/s/<id>` or `/my/<name>`. Any other host: the whole path without a trailing slash. The query is ignored.
  A link without a path (`/`) has nothing to pin: the checkbox is disabled and `portero rules match` prints
  `remember: not offered`.
- The rule is inserted at the top level right before the asking rule, or before the asking group. The rest of
  the file keeps its comments and formatting.
- The write is the same as in the UI: a lock on `rules.lock` in the config directory, a symlinked or offloaded
  file is refused, the new text must parse and route the link to the chosen target with every other rule
  unchanged, the file must not have changed since it was read, the previous file is kept as `rules.yaml.bak`,
  and the file is replaced atomically with mode 0600.
- A failed write never blocks the link: it opens first, the failure is logged.
- A remembered rule pins only the host and the meeting path. An `opener` or `cwd` condition of the asking rule is
  not copied, so after "Remember" that meeting opens in the chosen target from every app.
- A remembered rule names a browser of this Mac. On another Mac that shares the file through iCloud Drive,
  `portero rules check` warns when that browser is missing.
- A tool that installs `rules.yaml` from a template sees the file differ from the template once a rule has been
  remembered, and reports that as drift.

### The CLI never asks

`portero open` has no panel: for an asking rule it opens the `default` target and logs `asked -> default (CLI)`.
`portero rules match` and `portero open --dry-run` print the choices, the target Esc uses and the key a
"Remember" would pin:

```text
ask: [Dia/Personal, Dia/Work]  (rule: Meetings)
cancel: Dia/Personal (default)
remember: meet.google.com /abc-defg-hij
```

`--dry-run` adds one `would run (N):` line per choice and a `would run (Esc):` line.

## Working directory

`cwd` matches the working directory of the command that ran `portero open`. It routes links that command-line
tools open from a shell, for example login pages, when `$BROWSER` points to Portero:

```sh
export BROWSER="portero open"
```

```yaml
rules:
  - name: CLI logins from work repositories
    match: { cwd: ~/Documents/projects/work }
    open: { browser: Google Chrome, profile: Work }
```

- `cwd` takes one directory prefix or a list. A prefix is an absolute path, `~` or `~/...`; `~` is the home
  directory. A relative path or `~user` is an error in `portero rules check`.
- Prefixes are cleaned and resolved through symlinks when they exist.
- A prefix matches the directory itself and everything below it, on whole path segments: `/a/b` matches `/a/b`
  and `/a/b/c`, not `/a/bc`. `/` matches every directory.
- The comparison is case-insensitive on macOS (the default APFS volume is case-insensitive) and exact elsewhere.
- The working directory is the current directory of `portero open` and `portero rules match`, resolved to its
  physical path. `--cwd <dir>` replaces it; a `~` in the value is expanded by Portero, and a directory that does
  not exist is accepted, for what-if checks.
- Portero.app has no working directory: Launch Services starts it in `/`. Links from other apps never match a
  `cwd` rule; only links opened through the CLI do.
- `portero open --dry-run` and `portero rules match` print a `cwd: <dir>` line before the decision when the rules
  file has a `cwd` rule or `--cwd` was given. A plain `portero open` prints nothing new.
- When the working directory cannot be determined, `cwd` rules are skipped and the link still opens;
  `portero open` prints `portero: cwd: unknown (...); cwd rules skipped` to stderr.
- `decisions.log` does not record the working directory; the rule name shows which rule won.

## Containers (Firefox, Firefox Developer Edition, Zen)

```yaml
rules:
  - name: personal mail
    match: { host: mail.example.com }
    open: { browser: Firefox, profile: Personal, container: Personal }
  - name: team wiki
    match: { host: wiki.example.com }
    open: { browser: Zen, container: Work }   # no profile: the default profile of Zen
```

- The link is opened as `open -a <app> [-n --args -P <profile>] "ext+container:name=<container>&url=<url>"`.
  Both values are percent-encoded; only the RFC 3986 unreserved characters (`A-Z a-z 0-9 - _ . ~`) stay as is.
- The profile needs the extension "Open external links in a container" (addons.mozilla.org, id
  `{f069aec0-43c5-4bbf-b6b4-df95c4326b98}`).
- In Zen a Space can be bound to a container, so the link lands in the Space of that container.
- The name is matched case-insensitively against `containers.json` of the profile (the named one, else the one
  the `[Install...]` section of profiles.ini names with `Default=`, else the one marked `Default=1`, else the
  first); the spelling from the file is sent. A name that is not in `containers.json` is sent as written and the
  plan carries a note. The built-in containers
  are Personal, Work, Banking and Shopping under their English labels; a browser with a localized UI shows its own
  labels, so custom container names are the safer choice. An unknown name is created by the extension on first use.
- The link carries the container only when the extension is confirmed active in the resolved profile. Otherwise
  it opens without a container and the plan says why: the extension is off, `extensions.json` is missing or
  unreadable, the profile is not found, or the default profile is unknown because several `[Install...]`
  sections of profiles.ini name different profiles. A `file://` link or any scheme other than `http`/`https`
  also opens without a container.
- A `container` on any other browser is ignored at launch (the plan says `container "<name>" ignored`) and is an
  error in `portero rules check`.

`portero browsers` prints one line per Firefox-family profile whose `containers.json` is readable:

```text
Zen                          firefox   app.zen-browser.zen
    profiles: Personal, Work
    containers of Personal: Personal, Work, Banking, Shopping (extension: active)
```

The suffix is `(extension: active)` or `(extension: missing)`; it is left out when `extensions.json` cannot be
read.

## Local files

Portero also opens HTML documents handed to it by macOS (Open With, drag onto the icon). A `file://` URL has
no host and no opener, so rules are not consulted: it opens in `default`, logged with the rule name `file`.

## Sync through iCloud Drive

`portero sync icloud` moves the rules file to `iCloud Drive/Portero/rules.yaml` (or adopts the file already there)
and `portero sync off` copies it back. iCloud can offload that file: on current macOS it stays in place under its
own name but without data (reading it would block until it is downloaded); on older systems a placeholder
`.rules.yaml.icloud` stands in its place.

- `sync icloud` and `sync off` start the download and wait up to 30 seconds. When the file does not arrive they
  stop with an error and change nothing (the local file and the config stay as they were).
- `sync off` keeps the local file it replaces as `rules.yaml.icloud-backup` next to it, overwritten on every
  switch.
- Files are copied through a temporary file that is flushed to disk before the rename.
- The link handler never waits for a download: while the rules file is offloaded, a link opens in Safari at once,
  an `ERROR` line is added to `decisions.log` and the download is started, so later links are routed normally.
  The same applies to the address `portero serve` opens for the UI.
- `portero rules check` on an offloaded file starts the download and exits with an error; run it again after a
  few seconds.

## Log

Every routed link appends one tab-separated line to `~/.config/portero/decisions.log`:
time, opener, rule, target, url. The rule is the label from "Names and labels" (`Work / GitHub organisation`
for a rule in a group). The target is `Browser`, `Browser/Profile`, `Browser:Container` or
`Browser/Profile:Container`, written as in the rule (also when the container was skipped at launch).

An asking rule adds a sixth column with the outcome:

| Note | Meaning |
|---|---|
| `asked -> <target>` | the user picked this target in the panel |
| `asked -> <target>, remembered "<name>"` | the same, and "Remember" added the rule `<name>` |
| `asked -> default (cancelled)` | Esc, closing the panel, or the app quit; the `default` target was used |
| `asked -> default (CLI)` | `portero open` has no panel; the `default` target was used |

Lines of other rules have five columns. The target column holds the target that opened the link.
