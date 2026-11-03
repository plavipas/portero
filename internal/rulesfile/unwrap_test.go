package rulesfile

import (
	"errors"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

const unwrapBase = `# my rules
version: 1
default: {browser: Safari}
rules:
  # keep me
  - name: Code
    match: {host: [github.com]}
    open: {browser: Safari}
`

func TestSetUnwrap(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		unwrap bool
		skip   []string
		on     bool
	}{
		{"off", unwrapBase, false, nil, false},
		{"skip", unwrapBase, true, []string{"com.example.mail", "com.example.chat"}, true},
		{"off and skip", unwrapBase, false, []string{"com.example.mail"}, false},
		{"replace old keys", strings.Replace(unwrapBase, "default:", "unwrap: false\nunwrap_skip_openers:\n  - com.example.old\n- \ndefault:", 1), true, nil, true},
		{"replace block list at column 0", strings.Replace(unwrapBase, "rules:\n", "unwrap_skip_openers:\n- com.example.old\n- com.example.older\nrules:\n", 1), true, []string{"com.example.new"}, true},
		{"flow list", strings.Replace(unwrapBase, "rules:\n", "unwrap_skip_openers: [com.example.old]\nunwrap: false\nrules:\n", 1), true, nil, true},
		{"no trailing newline", strings.TrimSuffix(unwrapBase, "\n"), false, nil, false},
	}
	for _, c := range cases {
		in := strings.Replace(c.in, "- \n", "", 1)
		out, err := SetUnwrap(in, c.unwrap, c.skip)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		r, err := rules.Parse([]byte(out))
		if err != nil {
			t.Errorf("%s: result invalid: %v\n%s", c.name, err, out)
			continue
		}
		if on, _ := r.UnwrapFor(""); on != c.on {
			t.Errorf("%s: unwrap on=%v want %v", c.name, on, c.on)
		}
		if len(r.UnwrapSkipOpeners) != len(c.skip) {
			t.Errorf("%s: skip %v want %v", c.name, r.UnwrapSkipOpeners, c.skip)
		}
		if !strings.Contains(out, "# my rules") || !strings.Contains(out, "  # keep me") || len(r.Rules) != 1 {
			t.Errorf("%s: comments or rules lost:\n%s", c.name, out)
		}
	}
}

func TestSetUnwrapRoundTrip(t *testing.T) {
	out, err := SetUnwrap(unwrapBase, true, nil)
	if err != nil || out != unwrapBase {
		t.Errorf("defaults must leave the file byte for byte: %v\n%s", err, out)
	}
}

func TestSetUnwrapInvalid(t *testing.T) {
	if _, err := SetUnwrap("not: [valid", true, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v", err)
	}
	if _, err := SetUnwrap(unwrapBase, true, []string{"not a bundle id"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad skip: %v", err)
	}
}

func TestSetUnwrapKeepsComments(t *testing.T) {
	in := strings.Replace(unwrapBase, "rules:\n", "unwrap: false   # off for now\nunwrap_skip_openers:  # apps\n  # the mail app\n  - com.example.old\n  # indented note\n  - com.example.older\nrules:\n", 1)
	out, err := SetUnwrap(in, false, []string{"com.example.new"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unwrap: false  # off for now\n", "unwrap_skip_openers:  # apps\n", "# the mail app\n", "# indented note\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "com.example.old") {
		t.Errorf("old items kept:\n%s", out)
	}
	// the key is no longer written: its comment stays as a line of its own
	out, err = SetUnwrap(in, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# off for now\n") || strings.Contains(out, "unwrap:") {
		t.Errorf("default case:\n%s", out)
	}
}

func TestSetUnwrapColumnZeroCommentInList(t *testing.T) {
	in := strings.Replace(unwrapBase, "rules:\n", "unwrap_skip_openers:\n# note\n- com.example.old\nrules:\n", 1)
	out, err := SetUnwrap(in, true, []string{"com.example.new"})
	if err != nil || !strings.Contains(out, "com.example.new") || strings.Contains(out, "com.example.old") {
		t.Errorf("%v\n%s", err, out)
	}
}

func TestSetUnwrapCRLF(t *testing.T) {
	in := strings.ReplaceAll(unwrapBase, "\n", "\r\n")
	out, err := SetUnwrap(in, false, []string{"com.example.mail"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Errorf("bare LF in a CRLF file: %q", out)
	}
	if _, err := rules.Parse([]byte(out)); err != nil {
		t.Error(err)
	}
	// CRLF file without a final newline
	out, err = SetUnwrap(strings.TrimSuffix(in, "\r\n"), false, nil)
	if err != nil || strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Errorf("%v %q", err, out)
	}
}
