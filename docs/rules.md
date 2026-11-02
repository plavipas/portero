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
  One existing directory is enough, so a file shared between Macs can list the paths of each Mac.

Warnings do not fail the check (exit 0). A `container` on a browser outside the Firefox family is an error
(exit 1), and so is a `cwd` prefix that is not absolute and does not start with `~/`.

## Format

```yaml
version: 1
default: { browser: Dia, profile: Personal }
rules:
  - name: optional label, shown in the log
    match:
      host: example.com | [a.com, b.com]   # exact; "*.a.com" = any subdomain and a.com; "*" elsewhere = glob
      path: /prefix | [/a, /b]             # segment prefix: /a matches /a and /a/..., not /ab
      opener: com.vendor.app | [...]       # bundle id of the app that opened the link
      cwd: ~/dir | [/abs/dir, ...]         # working directory of a shell command; see "Working directory"
    open: { browser: <app name or bundle id>, profile: <optional>, container: <optional, Firefox family> }
```

- Rules are tried top to bottom; the first match wins; `default` applies when none does.
- Every key present under `match` must match; a rule needs at least one key.
- Host matching is case-insensitive; path matching is case-sensitive.
- `profile` means whatever the browser calls it: profile (Dia, Chrome, Firefox) or space (Arc). Name them the way
  you split your life, e.g. Personal and Work.
- `profile` and `container` must not contain a tab or a line break.

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
- A Portero build without `cwd` support rejects the whole rules file (unknown key). Update Portero on every Mac
  that reads the file before adding the first `cwd` rule.

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

## Log

Every routed link appends one tab-separated line to `~/.config/portero/decisions.log`:
time, opener, rule, target, url. The target is `Browser`, `Browser/Profile`, `Browser:Container` or
`Browser/Profile:Container`, written as in the rule (also when the container was skipped at launch).
