package rules

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const groupedFile = `
version: 1
default: {browser: Safari}
rules:
  - name: Work
    open: {browser: Dia, profile: Work}
    rules:
      - name: forge
        match: {host: git.example.com}
      - name: corp
        match: {host: "*.corp.example.net"}
      - name: glob
        match: {host: "docs-*.example.org"}
      - name: org
        match: {host: github.com, path: /work-org}
      - name: chat
        match: {opener: com.example.chat}
      - name: shell
        match: {cwd: "~/work"}
      - match: {host: tracker.example.com}
  - name: wiki
    match: {host: wiki.example.com}
    open: {browser: Zen, container: Team}
  - match: {host: github.com}
    open: {browser: Dia, profile: Personal}
`

const flatFile = `
version: 1
default: {browser: Safari}
rules:
  - {name: forge, match: {host: git.example.com}, open: {browser: Dia, profile: Work}}
  - {name: corp, match: {host: "*.corp.example.net"}, open: {browser: Dia, profile: Work}}
  - {name: glob, match: {host: "docs-*.example.org"}, open: {browser: Dia, profile: Work}}
  - {name: org, match: {host: github.com, path: /work-org}, open: {browser: Dia, profile: Work}}
  - {name: chat, match: {opener: com.example.chat}, open: {browser: Dia, profile: Work}}
  - {name: shell, match: {cwd: "~/work"}, open: {browser: Dia, profile: Work}}
  - {match: {host: tracker.example.com}, open: {browser: Dia, profile: Work}}
  - {name: wiki, match: {host: wiki.example.com}, open: {browser: Zen, container: Team}}
  - {match: {host: github.com}, open: {browser: Dia, profile: Personal}}
`

func mustParse(t *testing.T, text string) *Rules {
	t.Helper()
	r, err := Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestGroupedEqualsFlat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "work", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	inWork, err := ResolveDir(filepath.Join(home, "work", "sub"))
	if err != nil {
		t.Fatal(err)
	}
	elsewhere, _ := ResolveDir(filepath.Join(home, "other"))
	grouped, flat := mustParse(t, groupedFile), mustParse(t, flatFile)

	tests := []struct {
		name, link, opener, cwd string
		wantRule                string // label in the grouped file
	}{
		{"exact host", "https://git.example.com/a", "", "", "Work / forge"},
		{"subdomain", "https://a.corp.example.net/", "", "", "Work / corp"},
		{"bare domain of wildcard", "https://corp.example.net/", "", "", "Work / corp"},
		{"glob", "https://docs-eu.example.org/x", "", "", "Work / glob"},
		{"path inside", "https://github.com/work-org/repo", "", "", "Work / org"},
		{"path not a segment prefix", "https://github.com/work-organic", "", "", "#3"},
		{"other github", "https://github.com/someone", "", "", "#3"},
		{"opener", "https://other.example.org/", "com.example.chat", "", "Work / chat"},
		{"opener case", "https://other.example.org/", "COM.EXAMPLE.CHAT", "", "Work / chat"},
		{"cwd with tilde", "https://other.example.org/", "", inWork, "Work / shell"},
		{"cwd elsewhere", "https://other.example.org/", "", elsewhere, ""},
		{"no opener no cwd", "https://other.example.org/", "", "", ""},
		{"unnamed child", "https://tracker.example.com/", "", "", "Work"},
		{"plain rule after the group", "https://wiki.example.com/", "", "", "wiki"},
		{"non-http", "mailto:a@example.com", "", "", ""},
		{"first match wins in group", "https://git.example.com/", "com.example.chat", "", "Work / forge"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.link)
			if err != nil {
				t.Fatal(err)
			}
			req := Request{URL: u, Opener: tc.opener, Cwd: tc.cwd}
			g, f := grouped.Route(req), flat.Route(req)
			if g.Target != f.Target {
				t.Errorf("grouped %v, flat %v", g.Target, f.Target)
			}
			if g.Rule != tc.wantRule {
				t.Errorf("rule %q, want %q", g.Rule, tc.wantRule)
			}
			if wantGroup := strings.HasPrefix(tc.wantRule, "Work"); wantGroup != (g.Group == "Work") {
				t.Errorf("group %q for rule %q", g.Group, g.Rule)
			}
		})
	}
}

func TestGroupLabels(t *testing.T) {
	r := mustParse(t, groupedFile)
	if got := Label(0, r.Rules[0]); got != "Work" {
		t.Errorf("group label %q", got)
	}
	if got := Label(2, r.Rules[2]); got != "#3" {
		t.Errorf("unnamed plain rule label %q (position counts groups)", got)
	}
	if got := ChildLabel(r.Rules[0], r.Rules[0].Rules[0]); got != "Work / forge" {
		t.Errorf("child label %q", got)
	}
	if got := ChildLabel(r.Rules[0], r.Rules[0].Rules[6]); got != "Work" {
		t.Errorf("unnamed child label %q", got)
	}
	d := Decision{Target: Target{Browser: "Dia", Profile: "Work"}, Rule: "Work / forge", Group: "Work"}
	if got := d.String(); got != "Dia/Work  (rule: Work / forge)" {
		t.Errorf("String %q", got)
	}
}

func TestGroupUsesCwd(t *testing.T) {
	if !mustParse(t, groupedFile).UsesCwd() {
		t.Error("cwd inside a group is not seen")
	}
	if mustParse(t, strings.Replace(groupedFile, `match: {cwd: "~/work"}`, `match: {host: x.example.com}`, 1)).UsesCwd() {
		t.Error("UsesCwd without any cwd")
	}
}

func TestGroupValidation(t *testing.T) {
	head := "version: 1\ndefault: {browser: Safari}\nrules:\n"
	grp := "  - name: Work\n    open: {browser: Dia}\n    rules:\n"
	tests := []struct {
		name, rules, want string
	}{
		{"group without name", "  - open: {browser: Dia}\n    rules:\n      - match: {host: a.example.com}\n", `group #1 has no name`},
		{"group without browser", "  - name: Work\n    rules:\n      - match: {host: a.example.com}\n", `group "Work" has an empty browser`},
		{"group with match", "  - name: Work\n    open: {browser: Dia}\n    match: {host: a.example.com}\n    rules:\n      - match: {host: a.example.com}\n", `group "Work" has match; conditions go into its rules`},
		{"empty group", "  - name: Work\n    open: {browser: Dia}\n    rules: []\n", `group "Work" has no rules`},
		{"child without match", grp + "      - name: x\n        match: {}\n", `group "Work", rule "x" has no match conditions`},
		{"child with open by position", grp + "      - match: {host: a.example.com}\n      - match: {host: b.example.com}\n        open: {browser: Zen}\n", `group "Work", rule #2: open is set on the group only`},
		{"nested group", grp + "      - name: in\n        rules:\n          - match: {host: a.example.com}\n", `group "Work", rule "in": groups cannot contain groups`},
		{"child bad cwd", grp + "      - name: x\n        match: {cwd: relative/dir}\n", `group "Work", rule "x": cwd "relative/dir" must be`},
		{"group profile with spaces", "  - name: Work\n    open: {browser: Dia, profile: ' Work'}\n    rules:\n      - match: {host: a.example.com}\n", `group "Work": profile has leading or trailing spaces`},
		{"group name tab", "  - name: \"Wo\\trk\"\n    open: {browser: Dia}\n    rules:\n      - match: {host: a.example.com}\n", `name contains a control character`},
		{"child name newline", grp + "      - name: \"a\\nb\"\n        match: {host: a.example.com}\n", `name contains a control character`},
		{"child name spaces", grp + "      - name: ' x'\n        match: {host: a.example.com}\n", `name has leading or trailing spaces`},
		{"child name separator", grp + "      - name: 'a / b'\n        match: {host: a.example.com}\n", `name must not contain " / "`},
		{"plain name separator", "  - name: 'a / b'\n    match: {host: a.example.com}\n    open: {browser: Dia}\n", `name must not contain " / "`},
		{"plain name trailing space", "  - name: 'a '\n    match: {host: a.example.com}\n    open: {browser: Dia}\n", `name has leading or trailing spaces`},
		{"plain rule as before", "  - name: x\n    open: {browser: Dia}\n", `rule "x" has no match conditions`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(head + tc.rules))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestOldFlatExampleStillWorks(t *testing.T) {
	raw, err := os.ReadFile("testdata/flat_example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r := mustParse(t, string(raw))
	for _, rule := range r.Rules {
		if rule.IsGroup() {
			t.Errorf("%q became a group", rule.Name)
		}
	}
	u, _ := url.Parse("https://github.com/work-projects/x")
	d := r.Route(Request{URL: u})
	if d.Rule != "work repositories on GitHub" || d.Group != "" || d.Target.Profile != "Work" {
		t.Errorf("decision %+v", d)
	}
}
