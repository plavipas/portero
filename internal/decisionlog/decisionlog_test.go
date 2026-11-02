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
