package rules

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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

func TestTargetContainer(t *testing.T) {
	r, err := Parse([]byte("version: 1\ndefault: {browser: Zen, container: Work}\nrules:\n  - match: {host: example.com}\n    open: {browser: Zen, profile: Default, container: Alpha}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Default.Container != "Work" || r.Rules[0].Open.Container != "Alpha" {
		t.Errorf("parsed: %+v", r)
	}
	for _, c := range []struct {
		t    Target
		want string
	}{
		{Target{Browser: "Zen"}, "Zen"},
		{Target{Browser: "Zen", Profile: "Default"}, "Zen/Default"},
		{Target{Browser: "Zen", Container: "Work"}, "Zen:Work"},
		{Target{Browser: "Zen", Profile: "Default", Container: "Work"}, "Zen/Default:Work"},
	} {
		if got := c.t.String(); got != c.want {
			t.Errorf("String(%+v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestSurroundingSpaces(t *testing.T) {
	for name, text := range map[string]string{
		"container": "version: 1\ndefault: {browser: Zen}\nrules:\n  - name: x\n    match: {host: example.com}\n    open: {browser: Zen, container: \" Work\"}\n",
		"profile":   "version: 1\ndefault: {browser: Zen, profile: \"Default \"}\n",
	} {
		if _, err := Parse([]byte(text)); err == nil || !strings.Contains(err.Error(), name+" has leading or trailing spaces") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestControlCharacters(t *testing.T) {
	for name, text := range map[string]string{
		"container":         "version: 1\ndefault: {browser: Zen}\nrules:\n  - name: x\n    match: {host: example.com}\n    open: {browser: Zen, container: \"a\\tb\"}\n",
		"profile":           "version: 1\ndefault: {browser: Zen}\nrules:\n  - name: x\n    match: {host: example.com}\n    open: {browser: Zen, profile: \"a\\nb\"}\n",
		"default/container": "version: 1\ndefault: {browser: Zen, container: \"a\\rb\"}\n",
		"default/browser":   "version: 1\ndefault: {browser: \"Zen\\tX\"}\n",
	} {
		_, err := Parse([]byte(text))
		if err == nil || !strings.Contains(err.Error(), strings.TrimPrefix(name, "default/")+" contains a control character") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func cwdRules(t *testing.T, cwd ...string) *Rules {
	t.Helper()
	list := "["
	for i, c := range cwd {
		if i > 0 {
			list += ", "
		}
		list += `"` + c + `"`
	}
	list += "]"
	r, err := Parse([]byte("version: 1\ndefault: {browser: Safari}\nrules:\n  - name: cli\n    match: {cwd: " + list + "}\n    open: {browser: Dia, profile: Work}\n"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCwdRoute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(realHome, "work")
	if err := os.MkdirAll(filepath.Join(work, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(work, link); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://example.com/x")
	tests := []struct {
		name     string
		prefixes []string
		cwd      string
		want     bool
	}{
		{"exact", []string{"/a/b"}, "/a/b", true},
		{"below", []string{"/a/b"}, "/a/b/c", true},
		{"sibling with common prefix", []string{"/a/b"}, "/a/bc", false},
		{"parent", []string{"/a/b"}, "/a", false},
		{"trailing slash", []string{"/a/b/"}, "/a/b/c", true},
		{"root prefix", []string{"/"}, "/anything/at/all", true},
		{"tilde", []string{"~/work"}, work + "/repo", true},
		{"tilde alone", []string{"~"}, work, true},
		{"tilde other dir", []string{"~/work"}, realHome + "/other", false},
		{"symlinked prefix", []string{link}, work + "/repo", true},
		{"temp dir symlinks", []string{home + "/work"}, work, true},
		{"non-existent prefix", []string{"/no/such/dir"}, "/no/such/dir/sub", true},
		{"unknown cwd", []string{"/"}, "", false},
		{"second prefix", []string{"/x", "/a/b"}, "/a/b/c", true},
		{"no prefix matches", []string{"/x", "/y"}, "/a/b", false},
	}
	for _, tc := range tests {
		d := cwdRules(t, tc.prefixes...).Route(Request{URL: u, Cwd: tc.cwd})
		if got := d.Rule == "cli"; got != tc.want {
			t.Errorf("%s: matched = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCwdWithHost(t *testing.T) {
	r, err := Parse([]byte("version: 1\ndefault: {browser: Safari}\nrules:\n  - name: both\n    match: {host: example.com, cwd: /a}\n    open: {browser: Zen}\n"))
	if err != nil {
		t.Fatal(err)
	}
	ok, _ := url.Parse("https://example.com/")
	other, _ := url.Parse("https://example.org/")
	for _, tc := range []struct {
		u    *url.URL
		cwd  string
		want string
	}{{ok, "/a/b", "both"}, {ok, "/z", ""}, {other, "/a", ""}, {ok, "", ""}} {
		if got := r.Route(Request{URL: tc.u, Cwd: tc.cwd}).Rule; got != tc.want {
			t.Errorf("%v %q: rule %q, want %q", tc.u, tc.cwd, got, tc.want)
		}
	}
}

func TestCwdValidation(t *testing.T) {
	for name, cwd := range map[string]string{
		"relative": `"work/repo"`, "dot": `"./work"`, "other user": `"~bob/x"`, "empty": `""`, "control": `"/a\tb"`,
	} {
		text := "version: 1\ndefault: {browser: Safari}\nrules:\n  - name: x\n    match: {cwd: [" + cwd + "]}\n    open: {browser: Zen}\n"
		if _, err := Parse([]byte(text)); err == nil || !strings.Contains(err.Error(), `rule "x": cwd`) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := Parse([]byte("version: 1\ndefault: {browser: Safari}\nrules:\n  - name: x\n    match: {}\n    open: {browser: Zen}\n")); err == nil ||
		!strings.Contains(err.Error(), "(host, path, opener or cwd)") {
		t.Errorf("empty match: %v", err)
	}
	if _, err := Parse([]byte("version: 1\ndefault: {browser: Safari}\nrules:\n  - name: x\n    match: {cwd: [\"~\", \"~/a\", \"/\"]}\n    open: {browser: Zen}\n")); err != nil {
		t.Errorf("valid: %v", err)
	}
}

func TestUsesCwd(t *testing.T) {
	if !cwdRules(t, "/a").UsesCwd() {
		t.Error("cwd rule: UsesCwd false")
	}
	r, _ := Parse([]byte(sample))
	if r.UsesCwd() {
		t.Error("sample: UsesCwd true")
	}
}

func TestCwdMatchesFold(t *testing.T) {
	for _, tc := range []struct {
		prefix, dir string
		fold, want  bool
	}{
		{"/A/b", "/a/B/c", true, true},
		{"/A/b", "/a/B/c", false, false},
		{"/a/b", "/a/b", false, true},
		{"/a/b", "/a/bc", true, false},
	} {
		if got := CwdMatches(tc.prefix, tc.dir, tc.fold); got != tc.want {
			t.Errorf("CwdMatches(%q, %q, %v) = %v", tc.prefix, tc.dir, tc.fold, got)
		}
	}
}

func TestResolveDir(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "l")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(real)
	if got, err := ResolveDir(link); err != nil || got != want {
		t.Errorf("symlink: %q, %v; want %q", got, err, want)
	}
	if got, err := ResolveDir("/no/such/../dir/"); err != nil || got != "/no/dir" {
		t.Errorf("missing: %q, %v", got, err)
	}
}

func TestAskValidate(t *testing.T) {
	const head = "version: 1\ndefault: {browser: Safari}\nrules:\n"
	cases := []struct {
		name, body, want string // want "" means valid
	}{
		{"valid", "  - name: Meetings\n    match: {host: meet.example.com}\n    open:\n      ask:\n        - {browser: Dia, profile: Personal}\n        - {browser: Dia, profile: Work}\n", ""},
		{"empty ask", "  - name: M\n    match: {host: a.example.com}\n    open: {ask: []}\n", "ask is empty"},
		{"ask and browser", "  - name: M\n    match: {host: a.example.com}\n    open:\n      browser: Dia\n      ask: [{browser: Dia}]\n", "open has both ask and browser"},
		{"ask and profile only", "  - name: M\n    match: {host: a.example.com}\n    open:\n      profile: Work\n      ask: [{browser: Dia}]\n", "open has both ask and browser"},
		{"duplicate", "  - name: M\n    match: {host: a.example.com}\n    open: {ask: [{browser: Dia, profile: Work}, {browser: Dia, profile: Work}]}\n", "twice"},
		{"empty browser in ask", "  - name: M\n    match: {host: a.example.com}\n    open: {ask: [{profile: Work}]}\n", "empty browser"},
		{"control char", "  - name: M\n    match: {host: a.example.com}\n    open: {ask: [{browser: \"Dia\\t\"}]}\n", "control character"},
		{"ten targets", "  - name: M\n    match: {host: a.example.com}\n    open: {ask: [" + tenTargets() + "]}\n", "at most 9"},
		{"group asks", "  - name: G\n    open: {ask: [{browser: Dia}, {browser: Safari}]}\n    rules:\n      - {match: {host: a.example.com}}\n", ""},
		{"child asks", "  - name: G\n    open: {browser: Dia}\n    rules:\n      - match: {host: a.example.com}\n        open: {ask: [{browser: Dia}]}\n", "open is set on the group only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(head + c.body))
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("error = %v, want containing %q", err, c.want)
			}
		})
	}
}

func tenTargets() string {
	var s []string
	for i := 0; i < 10; i++ {
		s = append(s, fmt.Sprintf("{browser: B, profile: p%d}", i))
	}
	return strings.Join(s, ", ")
}

func TestAskInDefaultRejected(t *testing.T) {
	_, err := Parse([]byte("version: 1\ndefault:\n  ask: [{browser: Dia}]\nrules: []\n"))
	if err == nil {
		t.Fatal("ask in default must be rejected")
	}
}

func TestAskRoute(t *testing.T) {
	r, err := Parse([]byte(`version: 1
default: {browser: Dia, profile: Personal}
rules:
  - name: Meetings
    match: {host: meet.example.com}
    open:
      ask:
        - {browser: Dia, profile: Personal}
        - {browser: Dia, profile: Work}
  - name: Plain
    match: {host: plain.example.com}
    open: {browser: Safari}
  - name: Grp
    open: {ask: [{browser: Dia, profile: Work}]}
    rules:
      - name: room
        match: {host: grp.example.com}
`))
	if err != nil {
		t.Fatal(err)
	}
	route := func(raw string) Decision {
		u, _ := url.Parse(raw)
		return r.Route(Request{URL: u})
	}
	d := route("https://meet.example.com/abc-defg-hij")
	if len(d.Ask) != 2 || d.Target != r.Default || d.Rule != "Meetings" {
		t.Fatalf("decision = %+v", d)
	}
	if got, want := d.String(), "ask: [Dia/Personal, Dia/Work]  (rule: Meetings)"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if d := route("https://plain.example.com/"); len(d.Ask) != 0 || d.Target.Browser != "Safari" || d.String() != "Safari  (rule: Plain)" {
		t.Errorf("plain decision = %+v / %s", d, d)
	}
	if d := route("https://grp.example.com/"); len(d.Ask) != 1 || d.Group != "Grp" || d.Rule != "Grp / room" {
		t.Errorf("group decision = %+v", d)
	}
}

func TestRememberKey(t *testing.T) {
	cases := []struct {
		url, host, path string
		ok              bool
	}{
		{"https://meet.google.com/abc-defg-hij?authuser=1", "meet.google.com", "/abc-defg-hij", true},
		{"https://Meet.Google.com/abc-defg-hij/extra", "meet.google.com", "/abc-defg-hij", true},
		{"https://meet.google.com/lookup/xyz", "", "", false},
		{"https://meet.google.com/", "", "", false},
		{"https://us06web.zoom.us/j/123?pwd=x", "us06web.zoom.us", "/j/123", true},
		{"https://zoom.us/w/456/more", "zoom.us", "/w/456", true},
		{"https://example.zoom.us/s/789", "example.zoom.us", "/s/789", true},
		{"https://example.zoom.us/my/room", "example.zoom.us", "/my/room", true},
		{"https://example.zoom.us/profile/", "example.zoom.us", "/profile", true},
		{"https://example.com/team/room/", "example.com", "/team/room", true},
		{"https://example.com/a%2Fb", "example.com", "/a/b", true},                     // decoded path, the same one the matcher sees
		{"https://xn--bcher-kva.example/room", "xn--bcher-kva.example", "/room", true}, // punycode host kept as is
		{"https://zoom.us./j/123", "zoom.us.", "/j/123", true},                         // trailing dot kept: the rule matches exactly that host
		{"https://example.com/", "", "", false},
		{"https://example.com", "", "", false},
		{"https://example.com/a*b", "", "", false},
		{"https://example.com/a%20/%20b", "", "", false},
		{"https://example.com/a%09b", "", "", false},
		{"https://example.com/a%E2%80%AEb", "", "", false},        // U+202E right-to-left override
		{"https://us06%E2%80%8Bweb.zoom.us/j/123", "", "", false}, // U+200B zero-width space in the host
		{"https://example.com/a%E2%80%8Bb", "", "", false},        // U+200B zero width space
		{"https://example.com/a%E2%80%A8b", "", "", false},        // U+2028 line separator
		{"https://example.com/a%E2%80%A9b", "", "", false},        // U+2029 paragraph separator
		{"https://example.com/" + strings.Repeat("a", 600), "", "", false},
	}
	for _, c := range cases {
		u, err := url.Parse(c.url)
		if err != nil {
			t.Fatal(err)
		}
		host, path, ok := RememberKey(u)
		if host != c.host || path != c.path || ok != c.ok {
			t.Errorf("RememberKey(%s) = %q %q %v, want %q %q %v", c.url, host, path, ok, c.host, c.path, c.ok)
		}
	}
	if _, _, ok := RememberKey(nil); ok {
		t.Error("nil URL")
	}
}
