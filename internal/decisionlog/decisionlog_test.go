package decisionlog

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

func TestAppendContainerWithSpacesKeepsFiveColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.log")
	u, _ := url.Parse("https://example.com/a?b=1&c=2#top")
	d := rules.Decision{Rule: "wiki", Target: rules.Target{Browser: "Zen", Profile: "Default", Container: "Team Alpha Beta"}}
	if err := Append(path, d, rules.Request{URL: u}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSuffix(string(raw), "\n")
	cols := strings.Split(line, "\t")
	if len(cols) != 5 {
		t.Fatalf("columns = %d: %q", len(cols), line)
	}
	if cols[1] != "-" || cols[2] != "wiki" || cols[3] != "Zen/Default:Team Alpha Beta" || cols[4] != u.String() {
		t.Errorf("columns = %q", cols)
	}
}

func TestAppendGroupLabelKeepsFiveColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.log")
	u, _ := url.Parse("https://example.com/a")
	d := rules.Decision{Rule: "Work / GitHub", Group: "Work", Target: rules.Target{Browser: "Dia", Profile: "Work"}}
	if err := Append(path, d, rules.Request{URL: u}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cols := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\t")
	if len(cols) != 5 || cols[2] != "Work / GitHub" {
		t.Errorf("columns = %q", cols)
	}
}

func TestAppendNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.log")
	u, _ := url.Parse("https://meet.example.com/abc-defg-hij")
	req := rules.Request{URL: u, Opener: "com.apple.iCal"}
	d := rules.Decision{Target: rules.Target{Browser: "Dia", Profile: "Work"}, Rule: "Meetings"}
	if err := AppendNote(path, d, req, "asked -> Dia/Work, remembered \"a\tb\nc\""); err != nil {
		t.Fatal(err)
	}
	if err := AppendNote(path, d, req, ""); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines: %q", lines)
	}
	c := strings.Split(lines[0], "\t")
	if len(c) != 6 || c[5] != `asked -> Dia/Work, remembered "a b c"` || c[3] != "Dia/Work" {
		t.Errorf("columns: %q", c)
	}
	if len(strings.Split(lines[1], "\t")) != 5 {
		t.Errorf("no note must keep five columns: %q", lines[1])
	}
}

func TestAppendLinkUnwrapped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	link, _ := url.Parse("https://www.google.com/url?q=https%3A%2F%2Fgithub.com%2Fx")
	inner, _ := url.Parse("https://github.com/x")
	req := rules.Request{URL: inner, Opener: "com.example.mail"}
	if err := AppendLink(p, rules.Decision{Rule: "Code"}, req, link, "asked -> Dia/Work"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	c := strings.Split(strings.TrimSpace(string(b)), "\t")
	if len(c) != 6 || c[4] != link.String() || !strings.HasPrefix(c[5], "asked -> Dia/Work; unwrapped https://github.com/x") || strings.Contains(c[5], "\n") {
		t.Errorf("columns %q", c)
	}
}
