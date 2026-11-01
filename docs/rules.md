# Rules file

One YAML file, local to the Mac. Portero looks for it in this order:

1. `$PORTERO_RULES`
2. `rules: <path>` in `~/.config/portero/config.yaml`
3. `~/.config/portero/rules.yaml`

`portero rules path` prints the file in use, `portero rules check` validates it, `portero rules match <url>
[--opener <bundle id>]` shows the decision without opening anything.

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
    open: { browser: <app name or bundle id>, profile: <optional> }
```

- Rules are tried top to bottom; the first match wins; `default` applies when none does.
- Every key present under `match` must match; a rule needs at least one key.
- Host matching is case-insensitive; path matching is case-sensitive.
- `profile` means whatever the browser calls it: profile (Dia, Arc, Chrome), container or space.

## Log

Every routed link appends one tab-separated line to `~/.config/portero/decisions.log`:
time, opener, rule, target, url.
