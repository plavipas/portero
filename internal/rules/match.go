package rules

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// foldCwd: the default APFS volume is case-insensitive, so directories compare without case on macOS.
var foldCwd = runtime.GOOS == "darwin"

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// Request is one link to route.
type Request struct {
	URL    *url.URL
	Opener string // bundle id of the app that opened the link, or ""
	Cwd    string // physical absolute working directory (see ResolveDir), or "" when unknown
}

// Decision is the result of routing one link.
type Decision struct {
	Target Target
	// Ask lists the targets the handler app offers in a panel; Target is then the default target, which the
	// panel's Esc/close and every caller that cannot ask use. Empty for an ordinary decision.
	Ask   []Target
	Rule  string // label of the matched rule: "name", "#n" or "Group / name"; "" when the default applied
	Group string // name of the matched group; "" for a plain rule and for the default
}

// String renders "Browser/Profile  (rule: name)".
func (d Decision) String() string {
	rule := d.Rule
	if rule == "" {
		rule = "default"
	}
	if len(d.Ask) > 0 {
		names := make([]string, len(d.Ask))
		for i, t := range d.Ask {
			names[i] = t.String()
		}
		return fmt.Sprintf("ask: [%s]  (rule: %s)", strings.Join(names, ", "), rule)
	}
	return fmt.Sprintf("%s  (rule: %s)", d.Target, rule)
}

// Route returns the first matching rule's target, else the default. A group is walked in order; its first
// matching child decides and the group's target is used.
func (r *Rules) Route(req Request) Decision {
	for i, rule := range r.Rules {
		if rule.IsGroup() {
			for _, c := range rule.Rules {
				if c.Match.matches(req) {
					return r.decision(rule.Open, ChildLabel(rule, c), rule.Name)
				}
			}
			continue
		}
		if rule.Match.matches(req) {
			return r.decision(rule.Open, Label(i, rule), "")
		}
	}
	return Decision{Target: r.Default}
}

// decision builds the result for a matched element; an asking element falls back to the default target.
func (r *Rules) decision(o Open, label, group string) Decision {
	if o.Asks() {
		return Decision{Target: r.Default, Ask: o.Ask, Rule: label, Group: group}
	}
	return Decision{Target: o.Target, Rule: label, Group: group}
}

func (m Match) matches(req Request) bool {
	if len(m.Host) > 0 {
		host := strings.ToLower(req.URL.Hostname())
		if host == "" || !anyMatch(m.Host, func(p string) bool { return HostMatches(strings.ToLower(p), host) }) {
			return false
		}
	}
	if len(m.Path) > 0 {
		path := req.URL.Path
		if path == "" {
			path = "/"
		}
		if !anyMatch(m.Path, func(p string) bool { return PathMatches(p, path) }) {
			return false
		}
	}
	if len(m.Opener) > 0 {
		opener := strings.ToLower(req.Opener)
		if opener == "" || !anyMatch(m.Opener, func(p string) bool { return strings.ToLower(p) == opener }) {
			return false
		}
	}
	if len(m.Cwd) > 0 {
		if req.Cwd == "" || !anyMatch(m.Cwd, func(p string) bool { return CwdMatches(ExpandCwd(p), req.Cwd, foldCwd) }) {
			return false
		}
	}
	return true
}

// ResolveDir returns the absolute, symlink-free form of dir. A directory that does not exist is returned
// cleaned and absolute, without an error.
func ResolveDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return filepath.Clean(abs), nil
}

// ExpandCwd turns a cwd prefix from the rules file into a physical absolute path: `~` is the home directory,
// symlinks are resolved when the path exists. If the home directory is unknown the `~` stays and never matches.
func ExpandCwd(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		p = home + p[1:]
	}
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// CwdMatches is a prefix on path segments: `/a/b` matches `/a/b` and `/a/b/c`, not `/a/bc`; `/` matches all.
func CwdMatches(prefix, dir string, fold bool) bool {
	if fold {
		prefix, dir = strings.ToLower(prefix), strings.ToLower(dir)
	}
	if prefix == "/" {
		return true
	}
	return dir == prefix || strings.HasPrefix(dir, prefix+"/")
}

func anyMatch(patterns []string, f func(string) bool) bool {
	for _, p := range patterns {
		if f(p) {
			return true
		}
	}
	return false
}

// HostMatches: `example.com` exact; `*.example.com` any subdomain and the bare domain;
// any other `*` is a glob.
func HostMatches(pattern, host string) bool {
	if base, ok := strings.CutPrefix(pattern, "*."); ok && !strings.Contains(base, "*") {
		return host == base || strings.HasSuffix(host, "."+base)
	}
	if !strings.Contains(pattern, "*") {
		return host == pattern
	}
	return Glob(pattern, host)
}

// PathMatches is a prefix on path segments: `/org` matches `/org` and `/org/x`, not `/organic`.
// A trailing `*` or `/` on the prefix is ignored.
func PathMatches(prefix, path string) bool {
	p := strings.TrimSuffix(prefix, "*")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return true
	}
	return path == p || strings.HasPrefix(path, p+"/")
}

// Glob matches text against a pattern where `*` stands for any run of characters.
func Glob(pattern, text string) bool {
	p, t := []rune(pattern), []rune(text)
	pi, ti, star, mark := 0, 0, -1, 0
	for ti < len(t) {
		switch {
		case pi < len(p) && p[pi] == t[ti]:
			pi++
			ti++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, ti
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			ti = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}
