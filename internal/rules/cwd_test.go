package rules

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestExpandCwdWithoutHomeNeverMatches(t *testing.T) {
	t.Setenv("HOME", "")
	t.Chdir(t.TempDir())
	u, _ := url.Parse("https://example.com/")
	for _, prefix := range []string{"~", "~/work"} {
		got := ExpandCwd(prefix)
		if filepath.IsAbs(got) {
			t.Errorf("ExpandCwd(%q) without HOME = %q, want a non-absolute value", prefix, got)
		}
		for _, dir := range []string{"/", "/a/b", "/" + prefix} {
			if d := cwdRules(t, prefix).Route(Request{URL: u, Cwd: dir}); d.Rule == "cli" {
				t.Errorf("prefix %q without HOME matched cwd %q", prefix, dir)
			}
		}
	}
}

func TestExpandCwdDotDotIsCleaned(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://example.com/")
	r := cwdRules(t, base+"/a/b/../b")
	if d := r.Route(Request{URL: u, Cwd: base + "/a/b/c"}); d.Rule != "cli" {
		t.Errorf("prefix with .. did not match below its cleaned form: %+v", d)
	}
	if d := r.Route(Request{URL: u, Cwd: base + "/a"}); d.Rule == "cli" {
		t.Errorf("prefix with .. matched its parent: %+v", d)
	}
}

func TestCwdWithOpenerRequiresBoth(t *testing.T) {
	r, err := Parse([]byte("version: 1\ndefault: {browser: Safari}\nrules:\n  - name: both\n    match: {opener: com.example.Term, cwd: /a}\n    open: {browser: Zen}\n"))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://example.com/")
	for _, tc := range []struct {
		opener, cwd, want string
	}{
		{"com.example.term", "/a/b", "both"},
		{"com.example.Term", "/z", ""},
		{"com.example.Other", "/a", ""},
		{"", "/a", ""},
		{"com.example.Term", "", ""},
	} {
		if got := r.Route(Request{URL: u, Opener: tc.opener, Cwd: tc.cwd}).Rule; got != tc.want {
			t.Errorf("opener %q cwd %q: rule %q, want %q", tc.opener, tc.cwd, got, tc.want)
		}
	}
}

func TestCwdRuleOrderFirstMatchWins(t *testing.T) {
	r, err := Parse([]byte("version: 1\ndefault: {browser: Safari}\nrules:\n  - name: deep\n    match: {cwd: /a/b}\n    open: {browser: Zen}\n  - name: shallow\n    match: {cwd: /a}\n    open: {browser: Dia}\n"))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://example.com/")
	for cwd, want := range map[string]string{"/a/b/c": "deep", "/a/x": "shallow", "/b": ""} {
		if got := r.Route(Request{URL: u, Cwd: cwd}).Rule; got != want {
			t.Errorf("cwd %q: rule %q, want %q", cwd, got, want)
		}
	}
}
