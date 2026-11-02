package check

import (
	"errors"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

func testEnv(installed bool, list []browsers.Container, cerr error, active bool, aerr error) Env {
	return Env{
		Installed:  func(b browsers.Browser) bool { return installed || b.Name == "Safari" },
		Containers: func(browsers.Browser, string) (string, []browsers.Container, error) { return "Default", list, cerr },
		Extension:  func(browsers.Browser, string) (string, bool, error) { return "Default", active, aerr },
	}
}

func parse(t *testing.T, text string) *rules.Rules {
	t.Helper()
	r, err := rules.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCheckTargets(t *testing.T) {
	std := []browsers.Container{{Name: "Personal", ID: 1}, {Name: "Work", ID: 2}, {Name: "Banking", ID: 3}, {Name: "Shopping", ID: 4}}
	head := "version: 1\ndefault: {browser: Safari}\nrules:\n  - name: x\n    match: {host: example.com}\n    open: "
	tests := []struct {
		name    string
		open    string
		env     Env
		want    []string
		wantErr string
	}{
		{"fine", "{browser: Zen, container: work}", testEnv(true, std, nil, true, nil), nil, ""},
		{"no container needed", "{browser: Zen}", testEnv(true, nil, errors.New("x"), false, nil), nil, ""},
		{"not installed", "{browser: Zen, container: Work}", testEnv(false, std, nil, true, nil),
			[]string{`warning: rule "x": Zen is not installed on this Mac`}, ""},
		{"missing container", "{browser: Zen, container: Alpha}", testEnv(true, std, nil, true, nil),
			[]string{`warning: rule "x": profile "Default" of Zen has no container "Alpha" (have: Personal, Work, Banking, Shopping); the extension would create it on first use`}, ""},
		{"extension off", "{browser: Zen, container: Work}", testEnv(true, std, nil, false, nil),
			[]string{`warning: rule "x": profile "Default" of Zen has no active "Open external links in a container" extension; links open without a container`}, ""},
		{"unreadable files", "{browser: Zen, container: Work}", testEnv(true, nil, errors.New("x"), false, errors.New("y")),
			[]string{`warning: rule "x": the state of the "Open external links in a container" extension in Zen is unknown (profile not found or extensions.json unreadable); links open without a container`}, ""},
		{"unlisted app not installed", "{browser: Some Browser}", testEnv(false, std, nil, true, nil),
			[]string{`warning: rule "x": Some Browser is not installed on this Mac`}, ""},
		{"path as browser", "{browser: /Users/Shared/x.app}", testEnv(false, std, nil, true, nil),
			[]string{`warning: rule "x": browser "/Users/Shared/x.app" is a path; Portero opens it with open -a`}, ""},
		{"unlisted bundle id skipped", "{browser: com.example.browser}", testEnv(false, std, nil, true, nil), nil, ""},
		{"not firefox", "{browser: Dia, container: Work}", testEnv(true, std, nil, true, nil), nil,
			`rules: rule "x": container "Work" needs a Firefox-family browser (Firefox, Firefox Developer Edition, Zen); Dia does not take one`},
	}
	for _, tc := range tests {
		got, err := Targets(parse(t, head+tc.open+"\n"), tc.env)
		if tc.wantErr != "" {
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("%s: err = %v", tc.name, err)
			}
			continue
		}
		if err != nil || strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
}

func TestCheckTargetsDefaultAndUnnamed(t *testing.T) {
	r := parse(t, "version: 1\ndefault: {browser: Dia, container: Work}\n")
	if _, err := Targets(r, testEnv(true, nil, nil, true, nil)); err == nil || !strings.HasPrefix(err.Error(), `rules: default: container "Work"`) {
		t.Errorf("default: %v", err)
	}
	r = parse(t, "version: 1\ndefault: {browser: Safari}\nrules:\n  - match: {host: example.com}\n    open: {browser: Zen}\n")
	w, _ := Targets(r, testEnv(false, nil, nil, false, nil))
	if len(w) != 1 || !strings.HasPrefix(w[0], `warning: rule #1:`) {
		t.Errorf("unnamed: %q", w)
	}
}

func TestCheckCwd(t *testing.T) {
	head := "version: 1\ndefault: {browser: Safari}\nrules:\n  - name: x\n    match: {cwd: "
	tail := "}\n    open: {browser: Zen}\n  - match: {host: example.com}\n    open: {browser: Zen}\n"
	exists := func(have ...string) Env {
		return Env{DirExists: func(p string) bool {
			for _, h := range have {
				if p == h {
					return true
				}
			}
			return false
		}}
	}
	tests := []struct {
		name string
		cwd  string
		env  Env
		want []string
	}{
		{"none exists", "[~/Documents/projects/work, /opt/other]", exists(),
			[]string{`warning: rule "x": none of its cwd directories exist on this Mac (~/Documents/projects/work, /opt/other)`}},
		{"one exists", "[~/Documents/projects/work, /opt/other]", exists("/opt/other"), nil},
		{"single exists", "/opt/other", exists("/opt/other"), nil},
	}
	for _, tc := range tests {
		got := Cwd(parse(t, head+tc.cwd+tail), tc.env)
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
