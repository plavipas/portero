package serve

import (
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

func TestBuildGraph(t *testing.T) {
	r, err := rules.Parse([]byte(goodRules))
	if err != nil {
		t.Fatal(err)
	}
	ds := []Decision{
		mk("com.example.app", "work", "Safari/Work"),
		mk("com.example.app", "work", "Safari/Work"),
		mk("-", "default", "Safari"),
		mk("-", "old rule", "Zen:Work"),
		{Opener: "-", Rule: "ERROR", Kind: "error", Path: [3]string{"o:-", "s:error", "t:Safari"}},
	}
	g := BuildGraph(r, ds)
	if g.Total != 5 {
		t.Errorf("total %d", g.Total)
	}
	if len(g.Openers) != 2 || g.Openers[1].Label != "no opener / CLI" {
		t.Errorf("openers %+v", g.Openers)
	}
	ids := map[string]Node{}
	for _, n := range g.Rules {
		ids[n.ID] = n
	}
	for _, id := range []string{"r:work", "s:default", "s:file", "s:error", "r:old rule"} {
		if _, ok := ids[id]; !ok {
			t.Errorf("missing rule node %s", id)
		}
	}
	if !ids["r:old rule"].Stale || ids["r:work"].Stale {
		t.Errorf("stale flags: %+v", ids)
	}
	if g.Rules[0].ID != "r:work" {
		t.Errorf("file order first: %+v", g.Rules)
	}
	count := map[[2]string]int{}
	for _, e := range g.Edges {
		count[[2]string{e.From, e.To}] = e.Count
	}
	if count[[2]string{"o:com.example.app", "r:work"}] != 2 || count[[2]string{"r:work", "t:Safari/Work"}] != 2 {
		t.Errorf("counts %v", count)
	}
	if c, ok := count[[2]string{"s:file", "t:Safari"}]; !ok || c != 0 {
		t.Errorf("static edge missing: %v", count)
	}
	var zen Node
	for _, n := range g.Targets {
		if n.ID == "t:Zen:Work" {
			zen = n
		}
	}
	if zen.Browser != "Zen" || zen.Container != "Work" {
		t.Errorf("target fields %+v", zen)
	}
	g.markInstalled([]BrowserInfo{{Name: "Safari", BundleID: "com.apple.Safari", Installed: true}})
	for _, n := range g.Targets {
		if want := n.Browser == "Safari"; n.Installed != want {
			t.Errorf("%s installed=%v", n.ID, n.Installed)
		}
	}
}

func TestBuildGraphBrokenFileAndEmptyLog(t *testing.T) {
	g := BuildGraph(nil, []Decision{mk("-", "work", "Safari")})
	for _, n := range g.Rules {
		if n.Stale {
			t.Errorf("no file: nothing is stale (%s)", n.ID)
		}
	}
	g = BuildGraph(nil, nil)
	if g.Total != 0 || len(g.Openers) != 0 || len(g.Edges) != 1 { // only ERROR -> Safari
		t.Errorf("%+v", g)
	}
}

func mk(opener, rule, target string) Decision {
	d := Decision{Opener: opener, Rule: rule, Target: target, Kind: "ok"}
	d.Path = d.path()
	return d
}

func TestBuildGraphKeepsHostileLabelsAsData(t *testing.T) {
	d, ok := parseLine("2026-01-01T00:00:00Z\t<img src=x onerror=alert(1)>\t\"><script>x</script>\tSafari\thttps://example.com/<b>")
	if !ok {
		t.Fatal("line not parsed")
	}
	g := BuildGraph(nil, []Decision{d})
	if len(g.Openers) != 1 || g.Openers[0].Label != "<img src=x onerror=alert(1)>" {
		t.Errorf("opener label changed: %+v", g.Openers)
	}
	found := false
	for _, n := range g.Rules {
		if n.ID == `r:"><script>x</script>` && n.Stale == false {
			found = true
		}
	}
	if !found {
		t.Errorf("rule node from the log missing: %+v", g.Rules)
	}
}

func TestBuildGraphUserRuleNamedDefault(t *testing.T) {
	r, err := rules.Parse([]byte("version: 1\ndefault: {browser: Safari}\nrules:\n  - name: default\n    match: {host: example.com}\n    open: {browser: Safari, profile: Work}\n"))
	if err != nil {
		t.Fatal(err)
	}
	ds := []Decision{mk("-", "default", "Safari/Work")}
	g := BuildGraph(r, ds)
	ids := map[string]Node{}
	for _, n := range g.Rules {
		ids[n.ID] = n
	}
	if ids["r:default"].Kind != "rule" || ids["s:default"].Kind != "default" {
		t.Errorf("user rule and built-in default must be two nodes: %+v", g.Rules)
	}
	if ds[0].Path[1] != "r:default" {
		t.Errorf("a log line named like a user rule goes to the user rule: %v", ds[0].Path)
	}
	// without such a rule in the file the same log line is the built-in default
	g = BuildGraph(nil, []Decision{mk("-", "default", "Safari")})
	for _, n := range g.Rules {
		if n.ID == "r:default" {
			t.Errorf("unexpected user node: %+v", g.Rules)
		}
	}
}
