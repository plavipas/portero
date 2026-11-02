package rules

import (
	"net/url"
	"strings"
	"testing"
)

const orderedFile = `
version: 1
default: {browser: Safari}
rules:
  - name: early
    match: {host: early.example.com}
    open: {browser: Firefox}
  - name: Work
    open: {browser: Dia, profile: Work}
    rules:
      - name: exact
        match: {host: [ci.example.com, early.example.com]}
      - name: second
        match: {host: ci.example.com}
  - name: late
    match: {host: [ci.example.com, late.example.com]}
    open: {browser: Safari}
`

func TestGroupOrderingAgainstPlainRules(t *testing.T) {
	r := mustParse(t, orderedFile)
	tests := []struct {
		name, link, wantRule, wantGroup, wantBrowser string
	}{
		{"plain rule before the group wins", "https://early.example.com/", "early", "", "Firefox"},
		{"group wins over a later plain rule", "https://ci.example.com/", "Work / exact", "Work", "Dia"},
		{"later plain rule when no child matches", "https://late.example.com/", "late", "", "Safari"},
		{"default after everything", "https://none.example.com/", "", "", "Safari"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.link)
			if err != nil {
				t.Fatal(err)
			}
			d := r.Route(Request{URL: u})
			if d.Rule != tc.wantRule || d.Group != tc.wantGroup || d.Target.Browser != tc.wantBrowser {
				t.Errorf("decision %+v, want rule %q group %q browser %q", d, tc.wantRule, tc.wantGroup, tc.wantBrowser)
			}
		})
	}
}

func TestGroupHostMatchIsCaseInsensitive(t *testing.T) {
	r := mustParse(t, orderedFile)
	u, _ := url.Parse("https://CI.Example.COM/x")
	if d := r.Route(Request{URL: u}); d.Rule != "Work / exact" {
		t.Errorf("rule %q, want %q", d.Rule, "Work / exact")
	}
}

func TestGroupRejectsUnknownKeys(t *testing.T) {
	head := "version: 1\ndefault: {browser: Safari}\nrules:\n"
	tests := []struct{ name, rules, want string }{
		{"unknown key on the group", "  - name: Work\n    open: {browser: Dia}\n    colour: red\n    rules:\n      - match: {host: a.example.com}\n", "field colour not found"},
		{"unknown key on a child", "  - name: Work\n    open: {browser: Dia}\n    rules:\n      - match: {host: a.example.com}\n        hots: x\n", "field hots not found"},
		{"unknown key in a child match", "  - name: Work\n    open: {browser: Dia}\n    rules:\n      - match: {hots: a.example.com}\n", "field hots not found"},
		{"rules is a map, not a list", "  - name: Work\n    open: {browser: Dia}\n    rules: {match: {host: a.example.com}}\n", "cannot unmarshal"},
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

func TestGroupOpenControlCharacters(t *testing.T) {
	head := "version: 1\ndefault: {browser: Safari}\nrules:\n"
	tail := "    rules:\n      - match: {host: a.example.com}\n"
	tests := []struct{ name, open, want string }{
		{"tab in browser", `{browser: "Di\ta"}`, `group "Work": browser contains a control character`},
		{"newline in profile", `{browser: Dia, profile: "Wo\nrk"}`, `group "Work": profile contains a control character`},
		{"tab in container", `{browser: Zen, container: "Wo\trk"}`, `group "Work": container contains a control character`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(head + "  - name: Work\n    open: " + tc.open + "\n" + tail))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestGroupNullRulesWithoutMatchIsRefused(t *testing.T) {
	text := "version: 1\ndefault: {browser: Safari}\nrules:\n  - name: Work\n    open: {browser: Dia}\n    rules:\n"
	if _, err := Parse([]byte(text)); err == nil || !strings.Contains(err.Error(), "has no rules") {
		t.Errorf("error %v, want a refusal", err)
	}
}

func TestGroupDecisionOfDefaultHasNoGroup(t *testing.T) {
	r := mustParse(t, orderedFile)
	u, _ := url.Parse("https://none.example.com/")
	if d := r.Route(Request{URL: u}); d.Group != "" || d.Rule != "" || d.String() != "Safari  (rule: default)" {
		t.Errorf("decision %+v / %q", d, d.String())
	}
}
