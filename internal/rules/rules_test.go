package rules

import (
	"net/url"
	"strings"
	"testing"
)

const sample = `
version: 1
default:
  browser: Dia
  profile: Personal
rules:
  - name: company forge
    match:
      host: [gitlab.example.com, "*.example.net"]
    open: { browser: Zen }
  - name: work repositories on GitHub
    match:
      host: github.com
      path: /work-projects
    open: { browser: Dia, profile: Work }
  - name: work Slack links
    match:
      opener: com.tinyspeck.slackmacgap
    open: { browser: Dia, profile: Work }
  - name: wildcard host
    match:
      host: "docs-*.example.org"
    open: { browser: Safari }
`

func route(t *testing.T, raw, opener string) Decision {
	t.Helper()
	r, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return r.Route(Request{URL: u, Opener: opener})
}

func TestParse(t *testing.T) {
	r, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rules) != 4 {
		t.Fatalf("rules = %d", len(r.Rules))
	}
	if got := r.Rules[0].Match.Host; len(got) != 2 || got[1] != "*.example.net" {
		t.Errorf("host list = %v", got)
	}
	if r.Default != (Target{Browser: "Dia", Profile: "Personal"}) {
		t.Errorf("default = %+v", r.Default)
	}
}

func TestRoute(t *testing.T) {
	cases := []struct {
		url, opener string
		want        Target
		rule        string
	}{
		{"https://gitlab.example.com/x", "", Target{Browser: "Zen"}, "company forge"},
		{"https://jira.example.net/", "", Target{Browser: "Zen"}, "company forge"},
		{"https://example.net/", "", Target{Browser: "Zen"}, "company forge"},
		{"https://GITLAB.EXAMPLE.COM/", "", Target{Browser: "Zen"}, "company forge"},
		{"https://notexample.net/", "", Target{Browser: "Dia", Profile: "Personal"}, ""},
		{"https://github.com/work-projects/repo", "", Target{Browser: "Dia", Profile: "Work"}, "work repositories on GitHub"},
		{"https://github.com/work-projects", "", Target{Browser: "Dia", Profile: "Work"}, "work repositories on GitHub"},
		{"https://github.com/work-projects-2/repo", "", Target{Browser: "Dia", Profile: "Personal"}, ""},
		{"https://anything.io/", "com.tinyspeck.slackmacgap", Target{Browser: "Dia", Profile: "Work"}, "work Slack links"},
		{"https://anything.io/", "com.apple.mail", Target{Browser: "Dia", Profile: "Personal"}, ""},
		{"https://docs-v2.example.org/", "", Target{Browser: "Safari"}, "wildcard host"},
		{"https://docs.example.org/", "", Target{Browser: "Dia", Profile: "Personal"}, ""},
	}
	for _, c := range cases {
		d := route(t, c.url, c.opener)
		if d.Target != c.want || d.Rule != c.rule {
			t.Errorf("%s (opener %q): got %+v, want %+v rule %q", c.url, c.opener, d, c.want, c.rule)
		}
	}
}

func TestValidation(t *testing.T) {
	bad := map[string]string{
		"version":       "version: 2\ndefault: {browser: Dia}\nrules: []",
		"no match":      "version: 1\ndefault: {browser: Dia}\nrules:\n  - name: x\n    match: {}\n    open: {browser: Dia}",
		"no browser":    "version: 1\ndefault: {browser: Dia}\nrules:\n  - match: {host: a.com}\n    open: {profile: P}",
		"unknown key":   "version: 1\ndefault: {browser: Dia}\nrules:\n  - match: {hostname: a.com}\n    open: {browser: Dia}",
		"empty default": "version: 1\ndefault: {}\nrules: []",
	}
	for name, text := range bad {
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("%s: expected an error", name)
		} else if !strings.HasPrefix(err.Error(), "rules:") {
			t.Errorf("%s: error without prefix: %v", name, err)
		}
	}
}

func TestHelpers(t *testing.T) {
	if !PathMatches("/org/*", "/org/repo") || !PathMatches("/", "/anything") || PathMatches("/org", "/organic") {
		t.Error("PathMatches")
	}
	if !Glob("a*c", "abbc") || Glob("a*c", "abbd") || !Glob("*", "") || !HostMatches("*.a.com", "a.com") {
		t.Error("Glob/HostMatches")
	}
}
