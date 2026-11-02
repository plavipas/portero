package check

import (
	"reflect"
	"strings"
	"testing"
)

const groupText = `version: 1
default: {browser: Safari}
rules:
  - name: Work
    open: {browser: Zen, container: Work}
    rules:
      - name: a
        match: {cwd: /nonexistent/a}
      - match: {cwd: /nonexistent/b}
      - name: c
        match: {host: example.com}
  - name: plain
    match: {cwd: /nonexistent/p}
    open: {browser: Safari}
`

func noDirs() Env {
	e := testEnv(false, nil, nil, false, nil)
	e.DirExists = func(string) bool { return false }
	return e
}

func TestGroupTargetsCheckedOnce(t *testing.T) {
	r := parse(t, groupText)
	warnings, err := Targets(r, noDirs())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`warning: group "Work": Zen is not installed on this Mac`}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("warnings %q, want %q", warnings, want)
	}
}

func TestGroupContainerOnChromiumIsError(t *testing.T) {
	r := parse(t, strings.Replace(groupText, "browser: Zen", "browser: Dia", 1))
	_, err := Targets(r, noDirs())
	if err == nil || !strings.Contains(err.Error(), `group "Work": container "Work" needs a Firefox-family browser`) {
		t.Errorf("error %v", err)
	}
}

func TestGroupCwdWarnings(t *testing.T) {
	r := parse(t, groupText)
	got := Cwd(r, noDirs())
	want := []string{
		`warning: group "Work", rule "a": none of its cwd directories exist on this Mac (/nonexistent/a)`,
		`warning: group "Work", rule #2: none of its cwd directories exist on this Mac (/nonexistent/b)`,
		`warning: rule "plain": none of its cwd directories exist on this Mac (/nonexistent/p)`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("warnings %q, want %q", got, want)
	}
}

func TestDuplicateNames(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"group and plain rule", "  - name: Work\n    open: {browser: Safari}\n    rules:\n      - match: {host: a.example.com}\n  - name: Work\n    match: {host: b.example.com}\n    open: {browser: Safari}\n",
			[]string{`warning: name "Work" is used twice; the diagram and the log merge them`}},
		{"two plain rules, three times", "  - {name: x, match: {host: a.example.com}, open: {browser: Safari}}\n  - {name: x, match: {host: b.example.com}, open: {browser: Safari}}\n  - {name: x, match: {host: c.example.com}, open: {browser: Safari}}\n",
			[]string{`warning: name "x" is used twice; the diagram and the log merge them`}},
		{"same child names in different groups are fine", "  - name: A\n    open: {browser: Safari}\n    rules:\n      - {name: x, match: {host: a.example.com}}\n  - name: B\n    open: {browser: Safari}\n    rules:\n      - {name: x, match: {host: b.example.com}}\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := parse(t, "version: 1\ndefault: {browser: Safari}\nrules:\n"+tc.text)
			if got := Names(r); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("warnings %q, want %q", got, tc.want)
			}
			all, err := All(r, noDirs())
			if err != nil || len(all) != len(tc.want) {
				t.Errorf("All: %q, %v", all, err)
			}
		})
	}
}
