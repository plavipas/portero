package serve

import (
	"os"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

func TestSettingsAskUnmatched(t *testing.T) {
	r := settingsRig(t, Options{})
	code, m := r.get("/api/settings")
	a, _ := m["ask_unmatched"].(map[string]any)
	if code != 200 || a["enabled"] != true || a["editable"] != true || a["hash"] == "" {
		t.Fatalf("get: %d %v", code, m)
	}
	hash := a["hash"].(string)
	before, _ := os.ReadFile(r.rules)

	for _, body := range []map[string]any{
		{"enabled": false, "base_hash": "0000"},
		{"base_hash": hash},
		{"enabled": true, "extra": 1, "base_hash": hash},
	} {
		if code, m := r.post("/api/settings/ask-unmatched", body); code < 400 {
			t.Errorf("%v accepted: %d %v", body, code, m)
		}
	}
	if now, _ := os.ReadFile(r.rules); string(now) != string(before) {
		t.Fatal("a rejected request changed the file")
	}
	if code, _ := r.post("/api/settings/ask-unmatched", map[string]any{"enabled": false, "base_hash": "0000"}); code != 409 {
		t.Errorf("stale hash: %d", code)
	}

	code, m = r.post("/api/settings/ask-unmatched", map[string]any{"enabled": false, "base_hash": hash})
	if code != 200 || m["enabled"] != false || m["hash"] == "" || m["backup"] == "" {
		t.Fatalf("save: %d %v", code, m)
	}
	data, _ := os.ReadFile(r.rules)
	if !strings.Contains(string(data), "ask_unmatched: false\n") || !strings.HasPrefix(string(data), "# my rules\n") {
		t.Errorf("file:\n%s", data)
	}
	if bak, err := os.ReadFile(r.rules + ".bak"); err != nil || string(bak) != string(before) {
		t.Errorf("backup: %v", err)
	}
	if fi, _ := os.Stat(r.rules); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	_, m = r.get("/api/settings")
	if a, _ = m["ask_unmatched"].(map[string]any); a["enabled"] != false {
		t.Errorf("after: %v", a)
	}
	// the old hash is stale now; switching back removes the key
	if code, _ := r.post("/api/settings/ask-unmatched", map[string]any{"enabled": true, "base_hash": hash}); code != 409 {
		t.Errorf("stale: %d", code)
	}
	code, _ = r.post("/api/settings/ask-unmatched", map[string]any{"enabled": true, "base_hash": a["hash"]})
	if now, _ := os.ReadFile(r.rules); code != 200 || string(now) != string(before) {
		t.Errorf("back: %d\n%s", code, now)
	}
}

func TestSettingsAskUnmatchedNoFile(t *testing.T) {
	r := newRig(t, "")
	_, m := r.get("/api/settings")
	a, _ := m["ask_unmatched"].(map[string]any)
	if a["editable"] != false || a["note"] == "" {
		t.Errorf("%v", a)
	}
	if code, _ := r.post("/api/settings/ask-unmatched", map[string]any{"enabled": false, "base_hash": ""}); code < 400 {
		t.Errorf("no file: %d", code)
	}
}

func TestGraphFollowsDefaultAndBrief(t *testing.T) {
	rs, err := rules.Parse([]byte(`version: 1
default: {browser: Safari}
rules:
  - {name: Same, match: {host: same.example.com}, open: {default: true}}
`))
	if err != nil {
		t.Fatal(err)
	}
	g := BuildGraph(rs, nil)
	found := false
	for _, e := range g.Edges {
		if e.From == "r:Same" {
			found = true
			if !e.Follows || e.To != targetID("Safari") {
				t.Errorf("edge %+v", e)
			}
		}
	}
	if !found {
		t.Error("no edge for the follows-default rule")
	}
	if sum := summarize(rs); sum.Rules[0].Brief != "default" {
		t.Errorf("brief %q", sum.Rules[0].Brief)
	}
}
