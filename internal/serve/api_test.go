package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestState(t *testing.T) {
	r := newRig(t, goodRules)
	os.WriteFile(r.log, []byte(okLine(1)+okLine(2)), 0o600)
	code, m := r.get("/api/state")
	if code != 200 || m["hash"] != sum(goodRules) || m["text"] != goodRules || m["rules_place"] != "local" || m["rules_path"] != r.rules {
		t.Fatalf("%d %v", code, m)
	}
	if m["rules_error"] != nil || m["rules"].(map[string]any)["default"] != "Safari" {
		t.Errorf("summary: %v", m["rules"])
	}
	if len(m["decisions"].([]any)) != 2 || m["log_offset"].(float64) != float64(len(okLine(1)+okLine(2))) {
		t.Errorf("decisions %v offset %v", m["decisions"], m["log_offset"])
	}
	if m["graph"].(map[string]any)["total"].(float64) != 2 {
		t.Errorf("graph %v", m["graph"])
	}
}

func TestStateBrokenAndMissing(t *testing.T) {
	r := newRig(t, "version: 1\ndefault: {browser: Safari}\nrules:\n  - nope: 1\n")
	os.WriteFile(r.log, []byte(okLine(1)), 0o600)
	code, m := r.get("/api/state")
	if code != 200 || m["rules"] != nil || m["rules_error"] == nil || m["text"] == "" {
		t.Fatalf("broken: %d %v", code, m)
	}
	if m["rules_error"].(map[string]any)["line"].(float64) == 0 {
		t.Errorf("no line in %v", m["rules_error"])
	}
	if m["graph"].(map[string]any)["total"].(float64) != 1 {
		t.Error("graph from the log only")
	}
	r = newRig(t, "")
	code, m = r.get("/api/state")
	if code != 200 || m["exists"] != false || m["hash"] != "" || m["rules_error"] != nil {
		t.Errorf("missing: %d %v", code, m)
	}
	if len(m["decisions"].([]any)) != 0 {
		t.Error("empty log is an empty list")
	}
}

func TestValidate(t *testing.T) {
	r := newRig(t, goodRules)
	h := sum(goodRules)
	edited := strings.Replace(goodRules, "Work", "Personal", 1)
	code, m := r.post("/api/validate", map[string]any{"yaml": edited, "base_hash": h})
	if code != 200 || m["ok"] != true || m["stale"] != false || m["current_hash"] != h {
		t.Fatalf("%d %v", code, m)
	}
	changed := 0
	for _, l := range m["diff"].([]any) {
		if op := l.(map[string]any)["op"]; op != " " {
			changed++
		}
	}
	if changed != 2 {
		t.Errorf("diff %v", m["diff"])
	}
	_, m = r.post("/api/validate", map[string]any{"yaml": "version: 1\ndefault: {browser: Safari}\nrules:\n  - nope: 1\n", "base_hash": h})
	e := m["error"].(map[string]any)
	if m["ok"] != false || e["line"].(float64) != 4 {
		t.Errorf("error: %v", m)
	}
	_, m = r.post("/api/validate", map[string]any{"yaml": edited, "base_hash": "old"})
	if m["stale"] != true {
		t.Errorf("stale: %v", m)
	}
	_, m = r.post("/api/validate", map[string]any{"yaml": edited, "base_hash": ""})
	if m["stale"] != true {
		t.Errorf("empty base hash with a file is stale: %v", m)
	}
	_, m = r.post("/api/validate", map[string]any{"yaml": "version: 1\ndefault: {browser: Dia, container: Work}\n", "base_hash": h})
	if m["ok"] != false || !strings.Contains(m["error"].(map[string]any)["message"].(string), "Firefox") {
		t.Errorf("check error: %v", m)
	}
}

func TestValidateWarnings(t *testing.T) {
	r := newRig(t, goodRules)
	r.srv.opts.Env.Installed = func(b browsers.Browser) bool { return false }
	_, m := r.post("/api/validate", map[string]any{"yaml": goodRules, "base_hash": sum(goodRules)})
	w := m["warnings"].([]any)
	if m["ok"] != true || len(w) == 0 || !strings.Contains(w[0].(string), "not installed") {
		t.Errorf("%v", m)
	}
}

func doMatch(r *rig, v map[string]any) (int, map[string]any) { return r.post("/api/match", v) }

func TestMatch(t *testing.T) {
	text := `version: 1
default: {browser: Safari}
rules:
  - name: by host
    match: {host: example.com}
    open: {browser: Safari, profile: Work}
  - name: by path
    match: {host: github.example.com, path: /org}
    open: {browser: Safari, profile: Personal}
  - name: by opener
    match: {opener: com.example.chat}
    open: {browser: Safari, profile: Chat}
  - name: by cwd
    match: {cwd: CWD}
    open: {browser: Safari, profile: Cwd}
`
	cwdDir := t.TempDir()
	r := newRig(t, strings.Replace(text, "CWD", cwdDir, 1))
	var gotPlan rules.Target
	r.srv.opts.Plan = func(u *url.URL, tg rules.Target) (string, error) { gotPlan = tg; return "open " + u.Host, nil }
	tests := []struct {
		name string
		req  map[string]any
		rule string
		tgt  string
		note bool
	}{
		{"host", map[string]any{"url": "https://example.com/x"}, "by host", "Safari/Work", true},
		{"host and path", map[string]any{"url": "https://github.example.com/org/repo"}, "by path", "Safari/Personal", true},
		{"path miss", map[string]any{"url": "https://github.example.com/other"}, "default", "Safari", true},
		{"opener", map[string]any{"url": "https://other.example.org/", "opener": "com.example.chat"}, "by opener", "Safari/Chat", true},
		{"cwd", map[string]any{"url": "https://other.example.org/", "cwd": cwdDir + "/sub/dir"}, "by cwd", "Safari/Cwd", false},
		{"no cwd note", map[string]any{"url": "https://other.example.org/"}, "default", "Safari", true},
	}
	for _, tc := range tests {
		code, m := doMatch(r, tc.req)
		if code != 200 || m["rule"] != tc.rule || m["target"] != tc.tgt {
			t.Errorf("%s: %d %v", tc.name, code, m)
			continue
		}
		if (m["cwd_note"] != "") != tc.note {
			t.Errorf("%s: cwd_note %q", tc.name, m["cwd_note"])
		}
		if m["plan"] == nil {
			t.Errorf("%s: no plan", tc.name)
		}
	}
	if gotPlan.Browser != "Safari" {
		t.Errorf("plan target %+v", gotPlan)
	}
	if _, m := doMatch(r, map[string]any{"url": "https://example.com/", "opener": "com.example.chat"}); m["path"].([]any)[0] != "o:com.example.chat" {
		t.Errorf("path %v", m["path"])
	}
	// what-if on unsaved text, and a directory that does not exist
	what := "version: 1\ndefault: {browser: Safari}\nrules:\n  - match: {cwd: /nonexistent/x}\n    open: {browser: Safari, profile: Z}\n"
	_, m := doMatch(r, map[string]any{"yaml": what, "url": "https://a.example.com/", "cwd": "/nonexistent/x/y"})
	if m["rule"] != "#1" || m["cwd_used"] != "/nonexistent/x/y" {
		t.Errorf("what-if: %v", m)
	}
}

func TestMatchErrors(t *testing.T) {
	r := newRig(t, goodRules)
	if code, _ := doMatch(r, map[string]any{"url": "not a url"}); code != 400 {
		t.Errorf("no host: %d", code)
	}
	if code, _ := doMatch(r, map[string]any{"url": "https:///path"}); code != 400 {
		t.Errorf("empty host: %d", code)
	}
	if code, m := doMatch(r, map[string]any{"yaml": "version: 2\n", "url": "https://example.com/"}); code != 422 || m["error"] == nil {
		t.Errorf("bad yaml: %d %v", code, m)
	}
	if _, m := doMatch(r, map[string]any{"url": "https://example.com/"}); m["plan"] != nil {
		t.Error("no Plan option, no plan")
	}
}

func TestApply(t *testing.T) {
	r := newRig(t, goodRules)
	next := strings.Replace(goodRules, "Work", "Personal", 1) + "# kept comment\n"

	// invalid: 422, nothing written
	code, _ := r.post("/api/apply", map[string]any{"yaml": "version: 7\n", "base_hash": sum(goodRules)})
	if b, _ := os.ReadFile(r.rules); code != 422 || string(b) != goodRules {
		t.Fatalf("invalid: %d", code)
	}
	if _, err := os.Stat(r.rules + ".bak"); err == nil {
		t.Error("backup written for an invalid apply")
	}
	// stale: 409
	code, m := r.post("/api/apply", map[string]any{"yaml": next, "base_hash": "old"})
	if code != 409 || m["current_hash"] != sum(goodRules) {
		t.Fatalf("stale: %d %v", code, m)
	}
	if code, _ := r.post("/api/apply", map[string]any{"yaml": next, "base_hash": ""}); code != 409 {
		t.Errorf("empty base hash with a file: %d", code)
	}
	// ok
	code, m = r.post("/api/apply", map[string]any{"yaml": next, "base_hash": sum(goodRules)})
	if code != 200 || m["hash"] != sum(next) || m["backup"] != r.rules+".bak" {
		t.Fatalf("apply: %d %v", code, m)
	}
	if b, _ := os.ReadFile(r.rules); string(b) != next {
		t.Errorf("bytes written differ from the editor text:\n%s", b)
	}
	if b, _ := os.ReadFile(r.rules + ".bak"); string(b) != goodRules {
		t.Errorf("backup %q", b)
	}
	for _, p := range []string{r.rules, r.rules + ".bak"} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v %v", p, fi.Mode(), err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(r.dir, ".portero-*")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
	// applying again with the old hash is now stale
	if code, _ := r.post("/api/apply", map[string]any{"yaml": next, "base_hash": sum(goodRules)}); code != 409 {
		t.Errorf("second apply with old hash: %d", code)
	}
}

func TestApplyNewFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "portero")
	r := newRig(t, "")
	r.srv.opts.RulesFile = filepath.Join(dir, "rules.yaml")
	code, m := r.post("/api/apply", map[string]any{"yaml": goodRules, "base_hash": ""})
	if code != 200 || m["backup"] != "" {
		t.Fatalf("%d %v", code, m)
	}
	if _, err := os.Stat(r.srv.opts.RulesFile + ".bak"); err == nil {
		t.Error("no file before, so no .bak")
	}
	if fi, err := os.Stat(r.srv.opts.RulesFile); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v %v", fi, err)
	}
}

func TestApplyRefusesSymlink(t *testing.T) {
	r := newRig(t, "")
	target := filepath.Join(r.dir, "real.yaml")
	os.WriteFile(target, []byte(goodRules), 0o600)
	if err := os.Symlink(target, r.rules); err != nil {
		t.Fatal(err)
	}
	code, m := r.post("/api/apply", map[string]any{"yaml": goodRules + "# x\n", "base_hash": sum(goodRules)})
	if code != 409 || !strings.Contains(m["error"].(map[string]any)["message"].(string), "symlink") {
		t.Fatalf("%d %v", code, m)
	}
	if b, _ := os.ReadFile(target); string(b) != goodRules {
		t.Error("symlink target was written")
	}
	if fi, _ := os.Lstat(r.rules); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced")
	}
}

func TestDecisionsEndpoint(t *testing.T) {
	r := newRig(t, goodRules)
	os.WriteFile(r.log, []byte(okLine(1)+okLine(2)), 0o600)
	_, m := r.get("/api/state")
	off := int64(m["log_offset"].(float64))
	f, _ := os.OpenFile(r.log, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(okLine(3))
	f.Close()
	code, m := r.get("/api/decisions?offset=" + strconv.FormatInt(off, 10) + "&limit=5")
	if code != 200 || len(m["decisions"].([]any)) != 1 || m["truncated"] != false || m["offset"].(float64) != float64(off+int64(len(okLine(3)))) {
		t.Fatalf("%d %v", code, m)
	}
	os.WriteFile(r.log, []byte(okLine(9)), 0o600)
	_, m = r.get("/api/decisions?offset=" + strconv.FormatInt(off+1000, 10))
	if m["truncated"] != true || len(m["decisions"].([]any)) != 1 {
		t.Errorf("truncated: %v", m)
	}
	if code, m := r.get("/api/graph"); code != 200 || m["total"].(float64) != 1 {
		t.Errorf("graph %d %v", code, m)
	}
}

func TestApplyReplacesBackupSymlinkWithoutFollowingIt(t *testing.T) {
	r := newRig(t, goodRules)
	victim := filepath.Join(r.dir, "victim.txt")
	os.WriteFile(victim, []byte("keep"), 0o600)
	if err := os.Symlink(victim, r.rules+".bak"); err != nil {
		t.Fatal(err)
	}
	code, _ := r.post("/api/apply", map[string]any{"yaml": goodRules + "# x\n", "base_hash": sum(goodRules)})
	if code != 200 {
		t.Fatalf("apply: %d", code)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("symlink target of the backup was written: %q", b)
	}
	if fi, err := os.Lstat(r.rules + ".bak"); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("backup is still a symlink: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(r.rules + ".bak"); string(b) != goodRules {
		t.Errorf("backup %q", b)
	}
}

func TestApplyKeepsCRLFAndCommentsByteExact(t *testing.T) {
	r := newRig(t, goodRules)
	next := strings.ReplaceAll(goodRules, "\n", "\r\n") + "# trailing comment without newline"
	code, _ := r.post("/api/apply", map[string]any{"yaml": next, "base_hash": sum(goodRules)})
	if code != 200 {
		t.Fatalf("apply: %d", code)
	}
	if b, _ := os.ReadFile(r.rules); string(b) != next {
		t.Errorf("written %q", b)
	}
}

func TestDecisionsEndpointMissingLogAndGarbage(t *testing.T) {
	r := newRig(t, goodRules)
	code, m := r.get("/api/decisions?offset=0")
	if code != 200 || len(m["decisions"].([]any)) != 0 || m["truncated"] != false {
		t.Fatalf("missing log: %d %v", code, m)
	}
	os.WriteFile(r.log, []byte("garbage\n"+okLine(1)+"x\ty\n"), 0o600)
	_, m = r.get("/api/decisions?offset=0")
	if len(m["decisions"].([]any)) != 1 || m["skipped"].(float64) != 2 {
		t.Errorf("garbage: %v", m)
	}
	_, m = r.get("/api/state")
	if m["log_skipped"].(float64) != 2 {
		t.Errorf("state log_skipped %v", m["log_skipped"])
	}
}

func TestDecisionsEndpointLimitIsCapped(t *testing.T) {
	r := newRig(t, goodRules)
	var b strings.Builder
	for i := 0; i < 1500; i++ {
		b.WriteString(okLine(i))
	}
	os.WriteFile(r.log, []byte(b.String()), 0o600)
	_, m := r.get("/api/decisions?offset=0&limit=100000")
	if n := len(m["decisions"].([]any)); n != 1000 {
		t.Errorf("limit not capped at 1000: %d", n)
	}
}
