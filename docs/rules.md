# Rules file

One YAML file, local to the Mac. Portero looks for it in this order:

1. `$PORTERO_RULES`
2. `rules: <path>` in `~/.config/portero/config.yaml`
3. `~/.config/portero/rules.yaml`

`portero rules path` prints the file in use, `portero rules check` validates it, `portero rules match <url>
[--opener <bundle id>]` shows the decision without opening anything.

`portero rules check` also checks the targets against this Mac and prints a `warning:` line for:

- a browser that is not installed;
- a container that the profile does not have yet (the extension creates it on first use);
- a Firefox-family profile where the container extension is not active.

Warnings do not fail the check (exit 0). A `container` on a browser outside the Firefox family is an error
(exit 1).

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
    open: { browser: <app name or bundle id>, profile: <optional>, container: <optional, Firefox family> }
```

- Rules are tried top to bottom; the first match wins; `default` applies when none does.
- Every key present under `match` must match; a rule needs at least one key.
- Host matching is case-insensitive; path matching is case-sensitive.
- `profile` means whatever the browser calls it: profile (Dia, Chrome, Firefox) or space (Arc). Name them the way
  you split your life, e.g. Personal and Work.
- `profile` and `container` must not contain a tab or a line break.

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
  marked `Default=1` in profiles.ini, else the first); the spelling from the file is sent. The built-in containers
  are Personal, Work, Banking and Shopping under their English labels; a browser with a localized UI shows its own
  labels, so custom container names are the safer choice. An unknown name is created by the extension on first use.
- When `extensions.json` of the profile is readable and the extension is not active there, the link opens without
  a container. A `file://` link or any scheme other than `http`/`https` also opens without a container.
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
