package serve

import (
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

const groupRules = `version: 1
default: {browser: Safari}
rules:
  - name: Work
    open: {browser: Safari, profile: Work}
    rules:
      - name: a
        match: {host: a.example.com}
      - name: b
        match: {host: b.example.com}
      - match: {host: c.example.com}
  - name: plain
    match: {host: p.example.com}
    open: {browser: Safari, profile: Personal}
  - match: {host: u.example.com}
    open: {browser: Safari}
`

func TestGraphDrawsGroupAsOneNode(t *testing.T) {
	r, err := rules.Parse([]byte(groupRules))
	if err != nil {
		t.Fatal(err)
	}
	ds := []Decision{
		mk("-", "Work / a", "Safari/Work"),
		mk("-", "Work / a", "Safari/Work"),
		mk("com.example.app", "Work / b", "Safari/Work"),
		mk("-", "Work", "Safari/Work"),
		mk("-", "plain", "Safari/Personal"),
		mk("-", "#3", "Safari"),
	}
	g := BuildGraph(r, ds)
	var ids []string
	for _, n := range g.Rules {
		ids = append(ids, n.ID)
		if n.Stale {
			t.Errorf("stale node %+v", n)
		}
	}
	want := []string{"r:Work", "r:plain", "r:#3", "s:default", "s:none", "s:file", "s:error"}
	if len(ids) != len(want) {
		t.Fatalf("rule nodes %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("rule nodes %v, want %v", ids, want)
		}
	}
	if g.Rules[0].Label != "Work" || g.Rules[0].Kind != "rule" {
		t.Errorf("group node %+v", g.Rules[0])
	}
	count := map[[2]string]int{}
	for _, e := range g.Edges {
		count[[2]string{e.From, e.To}] = e.Count
	}
	if got := count[[2]string{"r:Work", "t:Safari/Work"}]; got != 4 {
		t.Errorf("group edge count %d, want 4 (a, a, b, group)", got)
	}
	if got := count[[2]string{"o:-", "r:Work"}]; got != 3 {
		t.Errorf("opener edge count %d, want 3", got)
	}
}

func TestLogRuleIDWithGroups(t *testing.T) {
	r, err := rules.Parse([]byte(groupRules))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ label, want string }{
		{"Work", "r:Work"},
		{"Work / a", "r:Work"},
		{"Work / b", "r:Work"},
		{"plain", "r:plain"},
		{"#3", "r:#3"},
		{"Work / gone", "r:Work / gone"},
		{"default", "s:default"},
		{"file", "s:file"},
		{"ERROR", "s:error"},
	}
	for _, tc := range tests {
		if got := logRuleID(r, tc.label); got != tc.want {
			t.Errorf("logRuleID(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

func TestMatchInsideGroup(t *testing.T) {
	r := newRig(t, groupRules)
	_, m := doMatch(r, map[string]any{"url": "https://b.example.com/x"})
	path, _ := m["path"].([]any)
	if m["rule"] != "Work / b" || len(path) != 3 || path[1] != "r:Work" || m["target"] != "Safari/Work" {
		t.Errorf("match %v", m)
	}
	_, m = doMatch(r, map[string]any{"url": "https://c.example.com/x"})
	path, _ = m["path"].([]any)
	if m["rule"] != "Work" || path[1] != "r:Work" {
		t.Errorf("unnamed child %v", m)
	}
	_, m = doMatch(r, map[string]any{"url": "https://u.example.com/x"})
	path, _ = m["path"].([]any)
	if m["rule"] != "#3" || path[1] != "r:#3" {
		t.Errorf("plain unnamed %v", m)
	}
}

func TestSummarizeGroup(t *testing.T) {
	r, err := rules.Parse([]byte(groupRules))
	if err != nil {
		t.Fatal(err)
	}
	sum := summarize(r)
	if len(sum.Rules) != 3 {
		t.Fatalf("top-level elements %d, want 3", len(sum.Rules))
	}
	g := sum.Rules[0]
	if !g.Group || g.Name != "Work" || g.Brief != "Safari/Work" || len(g.Rules) != 3 {
		t.Fatalf("group %+v", g)
	}
	if g.Rules[0].Name != "a" || g.Rules[0].Host[0] != "a.example.com" || g.Rules[2].Name != "#3" {
		t.Errorf("children %+v", g.Rules)
	}
	if sum.Rules[1].Group || sum.Rules[1].Rules != nil || sum.Rules[2].Name != "#3" {
		t.Errorf("plain rules %+v", sum.Rules[1:])
	}
}
