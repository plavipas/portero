package check

import (
	"reflect"
	"testing"
)

func TestShadowed(t *testing.T) {
	text := `version: 1
default: {browser: Safari}
rules:
  - name: "docs.example.com"
    match: {host: docs.example.com}
    open: {browser: Safari}
  - name: Wide
    match: {host: "*.example.com"}
    open: {browser: Safari}
  - name: Other
    match: {host: other.example.org}
    open: {browser: Safari}
  - name: G
    open: {browser: Safari}
    rules:
      - name: inner
        match: {host: [x.example.org, docs.example.com]}
  - name: narrow
    match: {host: docs.example.com, path: /a}
    open: {browser: Safari}
  - name: follow
    match: {host: f.example.org}
    open: {default: true}
`
	got := Shadowed(parse(t, text))
	want := []string{
		`warning: rule "docs.example.com" comes before rule "Wide" which also matches docs.example.com; Wide never sees docs.example.com links`,
		`warning: rule "docs.example.com" comes before rule "G / inner" which also matches docs.example.com; G / inner never sees docs.example.com links`,
		`warning: rule "docs.example.com" comes before rule "narrow" which also matches docs.example.com; narrow never sees docs.example.com links`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	// a host-only rule below the wide one shadows nothing; wildcard hosts are not exact
	clean := `version: 1
default: {browser: Safari}
rules:
  - {name: Wide, match: {host: "*.example.com"}, open: {browser: Safari}}
  - {name: docs.example.com, match: {host: docs.example.com}, open: {browser: Safari}}
`
	if got := Shadowed(parse(t, clean)); len(got) != 0 {
		t.Errorf("warnings %q", got)
	}
}

func TestTargetsSkipFollowsDefault(t *testing.T) {
	r := parse(t, `version: 1
default: {browser: Safari}
rules:
  - {name: A, match: {host: a.example.com}, open: {default: true}}
`)
	warnings, err := Targets(r, testEnv(true, nil, nil, true, nil))
	if err != nil || len(warnings) != 0 {
		t.Errorf("%q %v", warnings, err)
	}
}
